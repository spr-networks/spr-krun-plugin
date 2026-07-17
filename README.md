# spr-krun-plugin

Minimal guest-side base image for SPR plugins that run in a libkrun/KVM
microVM.

It adds one reusable guest component to SPR's existing `container_template`:
a statically linked AF_VSOCK-to-Unix-stream bridge, plus a small entrypoint
that starts it without capabilities. An ordinary plugin can therefore keep
its API on a guest-local Unix socket while libkrun exposes it as SPR's host
Unix socket.

The image installs no runtime packages. There is no DHCP client or server,
userspace network stack, IP listener, `passt`, or long-running network
configuration daemon in it. The dedicated host runtime enables libkrun's
embedded boot-time DHCP client, which configures virtio-net from SPR
CoreDHCP before this entrypoint starts.

Build the pinned local image with:

```bash
./build.sh
```

The repository and its GHCR package are private during development. Hosts
and CI jobs that build a derived plugin must authenticate to
`ghcr.io/spr-networks/spr-krun-plugin` before resolving the base image.

## Consumer contract

Build a plugin image on the base and leave its normal API on a guest-local
Unix socket:

```dockerfile
ARG SPR_KRUN_PLUGIN_REF=ghcr.io/spr-networks/spr-krun-plugin:latest
FROM ${SPR_KRUN_PLUGIN_REF}

COPY my-plugin /usr/local/bin/
CMD ["/usr/local/bin/my-plugin", "--unix-socket", "/run/spr-krun-plugin/plugin.sock"]
```

The krun Compose override supplies the runtime-specific environment and
annotations:

```yaml
services:
  my-plugin:
    runtime: krun-atlas
    network_mode: host
    networks: !reset []
    annotations:
      krun.tap_name: kmyplugin0
      krun.net_mac: "02:53:50:52:40:41"
      krun.vsock_path: /state/plugins/my-plugin/api/socket
      krun.vsock_port: "4040"
    environment:
      SPR_KRUN_PLUGIN_SOCKET: /run/spr-krun-plugin/plugin.sock
      SPR_KRUN_VSOCK_PORT: "4040"
    devices:
      - /dev/net/tun:/dev/net/tun
```

The matching SPR plugin manifest declares the VM as a normal device:

```json
{
  "NetworkCapabilities": {
    "Interface": "kmyplugin0",
    "DeviceMAC": "02:53:50:52:40:41",
    "Policies": ["wan", "dns"],
    "Groups": []
  }
}
```

The host path is:

```text
SPR Unix socket → libkrun vsock 4040 → spr-krun-vsock-proxy
                → guest-local plugin Unix socket
```

Guest egress is:

```text
plugin → guest virtio-net → host TAP → SPR CoreDHCP/router/firewall
```

The host runtime must add the TAP with libkrun's
`NET_FLAG_DHCP_CLIENT`. SPR's runtime patch gives the embedded client bounded
two-second retries because upstream's single 100 ms wait is too brittle for
an external router.

The vsock bridge only starts when both `SPR_KRUN_VSOCK_PORT` and
`SPR_KRUN_PLUGIN_SOCKET` are set, so the same derived image can retain a
plain runc/gVisor diagnostic mode.
