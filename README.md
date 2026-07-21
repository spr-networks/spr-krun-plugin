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

The repository and its multi-architecture GHCR package are public. Derived
plugins can resolve `ghcr.io/spr-networks/spr-krun-plugin` without registry
credentials.

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
    extends:
      file: docker-compose.yml
      service: my-plugin
    runtime: spr-krun
    annotations:
      krun.tap_name: kruntap0
      krun.net_uplink: eth0
      krun.net_mac: "02:53:50:52:40:41"
      krun.vsock_path: /state/plugins/my-plugin/api/socket
      krun.vsock_port: "4040"
      krun.vsock_connect_path: /state/api/eventbus.sock
      krun.vsock_connect_port: "4041"
    environment:
      SPR_KRUN_PLUGIN_SOCKET: /run/spr-krun-plugin/plugin.sock
      SPR_KRUN_VSOCK_PORT: "4040"
      SPR_KRUN_HOST_SOCKET: /run/spr-krun-plugin/eventbus.sock
      SPR_KRUN_HOST_VSOCK_PORT: "4041"
    devices:
      - /dev/net/tun:/dev/net/tun

networks:
  mypluginnet:
    name: spr-my-plugin
    driver_opts:
      com.docker.network.bridge.name: spr-my-plugin
```

The matching SPR plugin manifest declares the VM as a normal device:

```json
{
  "NetworkCapabilities": {
    "Interface": "spr-my-plugin",
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

A plugin can also reach an explicitly mapped host Unix socket without opening
an IP listener:

```text
guest-local Unix socket → spr-krun-vsock-proxy → libkrun vsock 4041
                        → host SPR Unix socket
```

Guest egress is:

```text
plugin → guest virtio-net → private TAP/bridge → Docker plugin network
       → SPR CoreDHCP/router/firewall
```

The host runtime must add the TAP with libkrun's
`NET_FLAG_DHCP_CLIENT`. SPR's runtime patch gives the embedded client bounded
two-second retries because upstream's single 100 ms wait is too brittle for
an external router.

The runtime refuses TAP networking without a private OCI network namespace.
It bridges the guest TAP to the Docker-provided `eth0` inside that namespace;
the VMM and plugin never join SPR's host network namespace.

Each direction starts only when both of its socket and port variables are set,
so the same derived image can retain a plain runc/gVisor diagnostic mode.
