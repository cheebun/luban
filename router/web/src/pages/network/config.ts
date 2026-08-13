import type { RouterConfig } from "../../api/index.ts";

export const DEFAULT_CONFIG: RouterConfig = {
  system: { hostname: "router", configured: true },
  wan: {
    mode: "dhcp",
    interface: "eth0",
    static: { address: "", gateway: "", dns: [] },
    pppoe: { username: "", password: "" },
    modem_ip: "auto",
    mtu: 0,
    mss: 0,
  },
  lan: {
    interfaces: [],
    address: "192.168.20.1/24",
    dhcp: {
      enabled: true,
      start: "192.168.20.100",
      end: "192.168.20.254",
      lease: "12h",
      dns_mode: "auto",
      dns_servers: [],
    },
  },
  ipv6: { enabled: true, lan_prefix_len: "auto" },
  dns: { upstreams: [], static_records: [] },
};
