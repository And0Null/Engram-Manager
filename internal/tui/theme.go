package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Palette mirrors the Pi "codec" theme for consistent visual identity.
var (
	// Colors
	colCyan         = lipgloss.Color("#42f2fa")
	colMagenta      = lipgloss.Color("#cf5af6")
	colGreen        = lipgloss.Color("#58e480")
	colYellow       = lipgloss.Color("#c1f44e")
	colRed          = lipgloss.Color("#f18150")
	colText         = lipgloss.Color("#dee4e4")
	colMuted        = lipgloss.Color("#bec8c9")
	colDim          = lipgloss.Color("#899393")
	colBorder       = lipgloss.Color("#14b0b8")
	colBorderAccent = lipgloss.Color("#c83bf7")
	colBorderMuted  = lipgloss.Color("#2e656b")
	colSelectedBg   = lipgloss.Color("#0d4144")
	colSurface      = lipgloss.Color("#1a2121")
	colSurfaceAlt   = lipgloss.Color("#252b2c")

	// Base text styles
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colCyan)

	styleAccent = lipgloss.NewStyle().
			Bold(true).
			Foreground(colMagenta)

	styleDim = lipgloss.NewStyle().
			Foreground(colDim)

	styleMuted = lipgloss.NewStyle().
			Foreground(colMuted)

	styleError = lipgloss.NewStyle().
			Bold(true).
			Foreground(colRed)

	styleSuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(colGreen)

	styleWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(colYellow)

	styleDeleted = lipgloss.NewStyle().
			Foreground(colDim).
			Strikethrough(true)

	stylePinned = lipgloss.NewStyle().
			Bold(true).
			Foreground(colYellow)

	// List row styles
	styleRowSelected = lipgloss.NewStyle().
				Bold(true).
				Background(colSelectedBg).
				Foreground(colText)

	styleRowNormal = lipgloss.NewStyle().
			Foreground(colText)

	// Panel borders (Lazygit-style rounded boxes)
	styleBoxActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colCyan).
			Padding(0, 1)

	styleBoxInactive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colBorderMuted).
				Padding(0, 1)

	styleBoxModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colMagenta).
			Padding(1, 2)

	// Header / Footer chips & pills
	styleBadge = lipgloss.NewStyle().
			Background(colSurfaceAlt).
			Foreground(colCyan).
			Padding(0, 1).
			Bold(true)

	styleBadgeMuted = lipgloss.NewStyle().
			Background(colSurface).
			Foreground(colDim).
			Padding(0, 1)

	styleBadgeWarn = lipgloss.NewStyle().
			Background(colSurfaceAlt).
			Foreground(colYellow).
			Padding(0, 1).
			Bold(true)

	styleKeyPill = lipgloss.NewStyle().
			Background(colBorderMuted).
			Foreground(colCyan).
			Padding(0, 1).
			Bold(true)

	styleKeyDesc = lipgloss.NewStyle().
			Foreground(colMuted).
			PaddingRight(1)
)

// renderKeyHelp renders a keybinding pill and its description like lazygit.
func renderKeyHelp(key, desc string) string {
	return styleKeyPill.Render(key) + " " + styleKeyDesc.Render(desc)
}
