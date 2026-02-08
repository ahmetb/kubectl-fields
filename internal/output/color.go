package output

import (
	"os"
	"strings"
)

// ANSI escape sequence constants.
const Reset = "\x1b[0m"

// BrightPalette contains 8 bright ANSI colors for manager name colorization.
// Colors are assigned to managers in insertion order.
var BrightPalette = []string{
	"\x1b[96m", // Bright Cyan
	"\x1b[92m", // Bright Green
	"\x1b[93m", // Bright Yellow
	"\x1b[95m", // Bright Magenta
	"\x1b[91m", // Bright Red
	"\x1b[94m", // Bright Blue
	"\x1b[36m", // Cyan (standard)
	"\x1b[33m", // Yellow (standard)
}

// ColorManager assigns ANSI colors to manager names in insertion order,
// cycling through the palette when all colors have been used.
type ColorManager struct {
	palette []string
	assign  map[string]int
	order   []string
}

// NewColorManager creates a ColorManager with the default BrightPalette.
func NewColorManager() *ColorManager {
	return &ColorManager{
		palette: BrightPalette,
		assign:  make(map[string]int),
	}
}

// ColorFor returns the ANSI escape code for the given manager name.
// On first encounter, the manager is assigned the next color in the palette
// (cycling if all colors are used). Subsequent calls return the same color.
func (cm *ColorManager) ColorFor(managerName string) string {
	if idx, ok := cm.assign[managerName]; ok {
		return cm.palette[idx%len(cm.palette)]
	}
	idx := len(cm.order)
	cm.assign[managerName] = idx
	cm.order = append(cm.order, managerName)
	return cm.palette[idx%len(cm.palette)]
}

// Wrap wraps text in the manager's assigned ANSI color code followed by reset.
func (cm *ColorManager) Wrap(text, managerName string) string {
	return cm.ColorFor(managerName) + text + Reset
}

// extractManagerName extracts the manager name from a comment string.
// The manager name is everything from start of the comment (after optional
// "# " prefix) up to the first " /" (subresource) or " (" (timestamp) or
// end of string.
func extractManagerName(comment string) string {
	s := comment
	// Strip leading "# " if present
	if strings.HasPrefix(s, "# ") {
		s = s[2:]
	}

	// Find first " /" (subresource delimiter) or " (" (timestamp delimiter)
	if idx := strings.Index(s, " /"); idx >= 0 {
		return s[:idx]
	}
	if idx := strings.Index(s, " ("); idx >= 0 {
		return s[:idx]
	}
	return s
}

// ResolveColor determines whether color output should be enabled based on
// the user's flag value and terminal state.
//
//   - "always": returns true (overrides everything including NO_COLOR)
//   - "never": returns false
//   - "auto": returns false if NO_COLOR env var is set and non-empty,
//     otherwise returns the isTTY parameter
func ResolveColor(flag string, isTTY bool) bool {
	switch flag {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if noColor := os.Getenv("NO_COLOR"); noColor != "" {
			return false
		}
		return isTTY
	}
}
