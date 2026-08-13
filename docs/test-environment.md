# Test Environment (PVE VM)

End-to-end validation environment used to field-test luban before real hardware
deployment.

## Topology

- **Hypervisor**: PVE at `10.19.0.2`
- **VM**: `106` (`luban-test`), Debian 13
- **WAN**: VM's `eth0` on `vmbr0`, gets a DHCP lease from the RouterOS VM upstream
- **LAN**: VM's `ens19` on an isolated bridge `br0` — `192.168.20.1` (the router itself)
- PVE host also holds `192.168.20.50` on `vmbr1` (the same isolated LAN segment),
  used as a client-side vantage point for testing the router's LAN behavior

## Accessing the Admin UI

The LAN segment isn't routable from a normal workstation, so tunnel through PVE:

```bash
ssh -N -L 8443:192.168.20.1:443 root@10.19.0.2
```

Then browse to `https://localhost:8443`. Login: `admin` / `password`.

## SSH Access to the VM

The WAN-side interface is firewalled (mirrors a real deployment — the router
doesn't expose SSH to WAN). Two ways in:

- `qm guest exec 106 -- <command>` from the PVE host (no network path needed)
- SSH ProxyCommand jump via the LAN gateway `192.168.20.1`

Test artifacts (logs, captured output, scratch files) are left in `/root` on
the VM.
