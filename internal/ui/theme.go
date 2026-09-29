package ui

// A small custom Fyne theme: dark background everywhere (independent of
// the OS light/dark setting) with a purple accent used for buttons,
// checkboxes, radio buttons, sliders, progress bars, and focus/selection
// highlights. Font, icon and sizing all fall back to Fyne's default theme
// — only colors are overridden.

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	purpleAccent  = color.NRGBA{R: 155, G: 89, B: 255, A: 255} // buttons, focus, progress bar fill
	purpleHover   = color.NRGBA{R: 130, G: 70, B: 220, A: 255}
	purpleDim     = color.NRGBA{R: 70, G: 45, B: 120, A: 255} // selection / pressed background
	darkBG        = color.NRGBA{R: 18, G: 16, B: 22, A: 255}  // window background
	darkPanel     = color.NRGBA{R: 28, G: 25, B: 34, A: 255}  // input/button base background
	darkSeparator = color.NRGBA{R: 55, G: 50, B: 65, A: 255}
	lightText     = color.NRGBA{R: 235, G: 232, B: 240, A: 255}
)

// darkPurpleTheme implements fyne.Theme.
type darkPurpleTheme struct{}

// NewDarkPurpleTheme returns the app's custom dark/purple theme.
func NewDarkPurpleTheme() fyne.Theme {
	return darkPurpleTheme{}
}

func (darkPurpleTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return darkBG
	case theme.ColorNameButton, theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return darkPanel
	case theme.ColorNameForeground:
		return lightText
	case theme.ColorNamePrimary:
		return purpleAccent
	case theme.ColorNameFocus:
		return purpleAccent
	case theme.ColorNameHover, theme.ColorNamePressed:
		return purpleHover
	case theme.ColorNameSelection:
		return purpleDim
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return darkSeparator
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 110, G: 105, B: 120, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 40, G: 37, B: 48, A: 255}
	case theme.ColorNameScrollBar:
		return purpleDim
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 150, G: 145, B: 160, A: 255}
	default:
		// Everything else (shadow, error, warning, success, ...) keeps
		// Fyne's own dark-variant color, so we don't have to hand-pick a
		// value for every single theme key.
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (darkPurpleTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (darkPurpleTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (darkPurpleTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
