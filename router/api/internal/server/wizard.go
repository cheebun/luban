package server

import (
	"context"
	"log/slog"
	"luban/internal/apply"
	"luban/internal/auth"
	"luban/internal/config"
	"luban/internal/detect"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// wizardProbeCache stores the results of the most recent POST /api/wizard/probe
// call. Thread-safe; shared across concurrent HTTP handlers.
type wizardProbeCache struct {
	mu      sync.RWMutex
	probed  bool
	results []detect.ProbeResult
}

func (c *wizardProbeCache) set(results []detect.ProbeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probed = true
	c.results = results
}

func (c *wizardProbeCache) get() (probed bool, results []detect.ProbeResult) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.probed, c.results
}

// wizardInterfaceJSON is the per-interface object in the wizard state/probe response.
type wizardInterfaceJSON struct {
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	MAC            string  `json:"mac"`
	Link           bool    `json:"link"`
	Wifi           bool    `json:"wifi"`
	RoleSuggestion *string `json:"role_suggestion"` // "wan"|"lan"|null
	DHCPOffer      *bool   `json:"dhcp_offer"`      // null until probed
	OfferServer    *string `json:"offer_server"`    // null when no offer
}

// wizardStateJSON is the GET /api/wizard/state response body.
type wizardStateJSON struct {
	Configured bool                  `json:"configured"`
	Board      *wizardBoardJSON      `json:"board"`
	Interfaces []wizardInterfaceJSON `json:"interfaces"`
	Probed     bool                  `json:"probed"`
}

type wizardBoardJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// handleWizardState handles GET /api/wizard/state.
// It re-reads live hardware on every call (board + interfaces); only the DHCP
// probe results are cached (from the last POST /api/wizard/probe).
func (s *Server) handleWizardState(w http.ResponseWriter, _ *http.Request) { //nolint:revive // r unused; required by http.Handler signature convention
	cfg := s.store.Get()
	probed, probeResults := s.probeCache.get()

	// Detect board (best-effort; nil on no match or I/O error).
	boardsDir := s.boardsDir()
	profiles, _ := detect.LoadBoards(boardsDir)
	board, _ := detect.MatchBoard(profiles)

	// Enumerate interfaces (best-effort; empty slice on error/non-Linux).
	ifaces, _ := detect.EnumerateInterfaces()

	resp := wizardStateJSON{
		Configured: cfg.System.Configured,
		Probed:     probed,
	}

	if board != nil {
		resp.Board = &wizardBoardJSON{ID: board.ID, Name: board.Name}
	}

	// Build a lookup for board interface specs by udev path.
	boardByPath := make(map[string]detect.InterfaceSpec)
	if board != nil {
		for _, spec := range board.Interfaces {
			if spec.Path != "" {
				boardByPath[spec.Path] = spec
			}
		}
	}

	// Build a lookup for probe results by interface name.
	probeByName := make(map[string]detect.ProbeResult)
	for _, pr := range probeResults {
		probeByName[pr.Interface] = pr
	}

	for _, iface := range ifaces {
		ji := wizardInterfaceJSON{
			Name: iface.Name,
			Path: iface.Path,
			MAC:  iface.MAC,
			Link: iface.LinkUp,
			Wifi: iface.Wifi,
		}

		// Role suggestion: board profile takes priority over probe results.
		if spec, ok := boardByPath[iface.Path]; ok && spec.Path != "" {
			role := spec.Role
			ji.RoleSuggestion = &role
		} else if probed {
			if pr, ok := probeByName[iface.Name]; ok {
				var role string
				if pr.GotOffer {
					role = "wan"
				} else {
					role = "lan"
				}
				ji.RoleSuggestion = &role
			}
		}

		// DHCP offer: only populated after a probe.
		if probed {
			if pr, ok := probeByName[iface.Name]; ok {
				gotOffer := pr.GotOffer
				ji.DHCPOffer = &gotOffer
				if pr.OfferServer != "" {
					s := pr.OfferServer
					ji.OfferServer = &s
				}
			} else {
				f := false
				ji.DHCPOffer = &f
			}
		}

		resp.Interfaces = append(resp.Interfaces, ji)
	}

	if resp.Interfaces == nil {
		resp.Interfaces = []wizardInterfaceJSON{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleWizardProbe handles POST /api/wizard/probe.
// Runs DHCP DISCOVER on all detected ethernet ports in parallel (~10s timeout),
// stores results in the probe cache, and returns the updated interface list in
// the same format as GET /api/wizard/state (interfaces array only).
func (s *Server) handleWizardProbe(w http.ResponseWriter, r *http.Request) {
	ifaces, err := detect.EnumerateInterfaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "enumerate interfaces: "+err.Error())
		return
	}

	// Collect ethernet-only names for probing (skip wifi).
	var etherNames []string
	for _, iface := range ifaces {
		if !iface.Wifi {
			etherNames = append(etherNames, iface.Name)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	slog.Info("wizard probe started", "interfaces", etherNames)
	probeResults, err := s.prober.Probe(ctx, etherNames)
	if err != nil {
		slog.Warn("wizard probe failed", "err", err)
		// Non-fatal: store empty results so the UI can continue.
		probeResults = make([]detect.ProbeResult, len(etherNames))
		for i, name := range etherNames {
			probeResults[i].Interface = name
		}
	}
	s.probeCache.set(probeResults)
	slog.Info("wizard probe complete", "count", len(probeResults))

	// Re-use handleWizardState to build the full response.
	s.handleWizardState(w, r)
}

// wizardCompleteReq is the POST /api/wizard/complete request body.
type wizardCompleteReq struct {
	WANInterface  string   `json:"wan_interface"`
	LANInterfaces []string `json:"lan_interfaces"`
	LANAddress    string   `json:"lan_address"` // optional; default 192.168.20.1/24
	Password      string   `json:"password"`
}

// handleWizardComplete handles POST /api/wizard/complete.
// Validates the request, writes config (configured=true, wan+lan+password),
// then triggers the apply pipeline. Returns the expected admin URL after
// the new config takes effect.
func (s *Server) handleWizardComplete(w http.ResponseWriter, r *http.Request) {
	var req wizardCompleteReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// ── Validation ────────────────────────────────────────────────────────────

	if req.WANInterface == "" {
		writeError(w, http.StatusUnprocessableEntity, "wan_interface is required")
		return
	}
	if len(req.LANInterfaces) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "lan_interfaces must not be empty")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusUnprocessableEntity, "password is required")
		return
	}

	// wan_interface must not appear in lan_interfaces.
	for _, lan := range req.LANInterfaces {
		if lan == req.WANInterface {
			writeError(w, http.StatusUnprocessableEntity, "wan_interface must not be in lan_interfaces")
			return
		}
	}

	// Interface names must exist on the system.
	knownIfaces, _ := detect.EnumerateInterfaces()
	knownSet := make(map[string]bool, len(knownIfaces))
	for _, iface := range knownIfaces {
		knownSet[iface.Name] = true
	}
	// Tolerate missing sysfs (non-Linux dev env) by only checking if we got results.
	if len(knownIfaces) > 0 {
		if !knownSet[req.WANInterface] {
			writeError(w, http.StatusUnprocessableEntity, "wan_interface not found: "+req.WANInterface)
			return
		}
		for _, lan := range req.LANInterfaces {
			if !knownSet[lan] {
				writeError(w, http.StatusUnprocessableEntity, "lan_interface not found: "+lan)
				return
			}
		}
	}

	lanAddress := req.LANAddress
	if lanAddress == "" {
		lanAddress = "192.168.20.1/24"
	}
	if _, _, err := net.ParseCIDR(lanAddress); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "lan_address is not a valid CIDR: "+err.Error())
		return
	}

	// ── Hash new password ─────────────────────────────────────────────────────

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password: "+err.Error())
		return
	}

	// ── Update config ─────────────────────────────────────────────────────────

	if err := s.store.SetField(func(c *config.Config) {
		c.System.Configured = true
		c.System.Admin.PasswordHash = hash
		c.System.Admin.MustChange = false
		c.WAN.Mode = "dhcp"
		c.WAN.Interface = req.WANInterface
		c.LAN.Interfaces = req.LANInterfaces
		c.LAN.Address = lanAddress
		c.LAN.DHCP.Enabled = true
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}

	// ── Trigger apply pipeline ────────────────────────────────────────────────

	cfg := s.store.Get()
	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "build template data: "+err.Error())
		return
	}
	if err := apply.Pipeline(r.Context(), s.baseDir, data); err != nil {
		slog.Error("wizard apply failed", "err", err)
		writeError(w, http.StatusInternalServerError, "apply failed: "+err.Error())
		return
	}

	// ── Derive the new admin URL ───────────────────────────────────────────────

	lanIP, _, _ := net.ParseCIDR(lanAddress)
	newURL := "https://" + lanIP.String() + "/"

	slog.Info("wizard complete", "wan", req.WANInterface, "lan", strings.Join(req.LANInterfaces, ","), "url", newURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "new_url": newURL})
}

// boardsDir returns the path to the shipped board profiles directory.
func (s *Server) boardsDir() string {
	// baseDir is /opt/router; boards/ is a sibling of config.json.
	// filepath.Join handles the path correctly.
	return s.baseDir + "/boards"
}
