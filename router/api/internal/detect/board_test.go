package detect_test

import (
	"luban/internal/detect"
	"os"
	"path/filepath"
	"testing"
)

// realBoardsDir locates router/boards/ relative to this test file.
// This file lives at router/api/internal/detect/, so router/boards/ is
// four levels up plus "boards".
func realBoardsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "boards")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("router/boards/ not found at %s (repo layout unavailable): %v", dir, err)
	}
	abs, _ := filepath.Abs(dir)
	return abs
}

func mustWriteDMI(t *testing.T, dir string, fields map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G301: test fixture directory
		t.Fatalf("mkdir dmi dir: %v", err)
	}
	for name, value := range fields {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o644); err != nil { //nolint:gosec // G306: test fixture, world-readable is appropriate
			t.Fatalf("write dmi fixture %s: %v", name, err)
		}
	}
}

func setDMIPath(t *testing.T, path string) {
	t.Helper()
	old := detect.DMIPath
	detect.DMIPath = path
	t.Cleanup(func() { detect.DMIPath = old })
}

func setDTModelPath(t *testing.T, path string) {
	t.Helper()
	old := detect.DTModelPath
	detect.DTModelPath = path
	t.Cleanup(func() { detect.DTModelPath = old })
}

// TestMatchBoard_DMIHit exercises the real minisforum-ms-a2 profile with
// a fixture that provides the exact DMI values the profile requires.
func TestMatchBoard_DMIHit(t *testing.T) {
	dmiDir := t.TempDir()
	mustWriteDMI(t, dmiDir, map[string]string{
		"sys_vendor":   "Micro Computer (HK) Tech Limited",
		"product_name": "MS-A2",
		"board_name":   "F1WSA",
	})
	setDMIPath(t, dmiDir)

	boardsDir := realBoardsDir(t)
	profiles, err := detect.LoadBoards(boardsDir)
	if err != nil {
		t.Fatalf("LoadBoards: %v", err)
	}

	board, err := detect.MatchBoard(profiles)
	if err != nil {
		t.Fatalf("MatchBoard: %v", err)
	}
	if board == nil {
		t.Fatal("expected board match, got nil")
	}
	if board.ID != "minisforum-ms-a2" {
		t.Errorf("board.ID = %q, want minisforum-ms-a2", board.ID)
	}
	if len(board.Interfaces) == 0 {
		t.Error("matched board has no interfaces")
	}
}

// TestMatchBoard_DMIMiss verifies that a board is NOT matched when a required
// DMI field differs from the profile's expected value.
func TestMatchBoard_DMIMiss(t *testing.T) {
	dmiDir := t.TempDir()
	mustWriteDMI(t, dmiDir, map[string]string{
		"sys_vendor":   "Some Other Vendor",
		"product_name": "Unknown",
		"board_name":   "XYZ",
	})
	setDMIPath(t, dmiDir)

	boardsDir := realBoardsDir(t)
	profiles, err := detect.LoadBoards(boardsDir)
	if err != nil {
		t.Fatalf("LoadBoards: %v", err)
	}

	board, err := detect.MatchBoard(profiles)
	if err != nil {
		t.Fatalf("MatchBoard: %v", err)
	}
	if board != nil {
		t.Errorf("expected no match, got board %q", board.ID)
	}
}

// TestLoadBoards_BoardsDPrecedence verifies that boards.d/ profiles are tried
// before boards/ profiles. A boards.d/ profile that matches the same hardware
// should shadow the boards/ profile.
func TestLoadBoards_BoardsDPrecedence(t *testing.T) {
	// Build a fake boards/ directory with one profile.
	boardsDir := t.TempDir()
	//nolint:gosec // G306: test fixture, world-readable JSON is appropriate
	if err := os.WriteFile(filepath.Join(boardsDir, "base.json"), []byte(`{
		"id": "base-board",
		"name": "Base Board",
		"match": {"dmi": {"sys_vendor": "ACME"}},
		"interfaces": []
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a fake boards.d/ directory that overrides the same match.
	boardsDDir := boardsDir + ".d"
	if err := os.MkdirAll(boardsDDir, 0o750); err != nil { //nolint:gosec // G301: test fixture directory
		t.Fatal(err)
	}
	//nolint:gosec // G306: test fixture, world-readable JSON is appropriate
	if err := os.WriteFile(filepath.Join(boardsDDir, "override.json"), []byte(`{
		"id": "override-board",
		"name": "Override Board",
		"match": {"dmi": {"sys_vendor": "ACME"}},
		"interfaces": []
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set DMI to match vendor "ACME".
	dmiDir := t.TempDir()
	mustWriteDMI(t, dmiDir, map[string]string{"sys_vendor": "ACME"})
	setDMIPath(t, dmiDir)

	profiles, err := detect.LoadBoards(boardsDir)
	if err != nil {
		t.Fatalf("LoadBoards: %v", err)
	}

	board, err := detect.MatchBoard(profiles)
	if err != nil {
		t.Fatalf("MatchBoard: %v", err)
	}
	if board == nil {
		t.Fatal("expected a match, got nil")
	}
	// boards.d/ profiles are placed first; the override should win.
	if board.ID != "override-board" {
		t.Errorf("board.ID = %q, want override-board (boards.d/ must take precedence)", board.ID)
	}
}

// TestMatchBoard_NoBoards verifies that an empty profiles slice returns nil.
func TestMatchBoard_NoBoards(t *testing.T) {
	setDMIPath(t, t.TempDir()) // empty dmi dir
	board, err := detect.MatchBoard(nil)
	if err != nil {
		t.Fatalf("MatchBoard(nil): %v", err)
	}
	if board != nil {
		t.Errorf("expected nil board for empty profiles, got %q", board.ID)
	}
}

// TestMatchBoard_DTModel verifies that a DT-model match works when
// the dt_model value is a substring of the real device-tree model string.
func TestMatchBoard_DTModel(t *testing.T) {
	dtFile := filepath.Join(t.TempDir(), "model")
	if err := os.WriteFile(dtFile, []byte("Raspberry Pi 4 Model B Rev 1.2\x00"), 0o644); err != nil { //nolint:gosec // G306: test fixture
		t.Fatal(err)
	}
	setDTModelPath(t, dtFile)
	setDMIPath(t, t.TempDir()) // empty dmi — force DT path

	profiles := []detect.BoardProfile{
		{
			ID:   "rpi4",
			Name: "Raspberry Pi 4",
			Match: detect.Match{
				DTModel: "Raspberry Pi 4",
			},
		},
	}

	board, err := detect.MatchBoard(profiles)
	if err != nil {
		t.Fatalf("MatchBoard: %v", err)
	}
	if board == nil {
		t.Fatal("expected DT model match, got nil")
	}
	if board.ID != "rpi4" {
		t.Errorf("board.ID = %q, want rpi4", board.ID)
	}
}
