// Package ui — the SHEYTAN fire theme.
//
// Design language: "Forge Dark". Near-black backgrounds with a warm red
// cast, ember-orange accents (#FF5A26), flame gradients on primary actions,
// and soft ember hovers. Every color is tuned for long developer sessions:
// high contrast where it matters, muted chrome everywhere else.
package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Fire palette — the single source of truth for every UI color.
var (
	ColBg         = color.NRGBA{R: 13, G: 7, B: 7, A: 255}      // #0D0707 window background
	ColBgRaised   = color.NRGBA{R: 24, G: 12, B: 11, A: 255}    // #180C0B panels/cards
	ColBgDeep     = color.NRGBA{R: 8, G: 4, B: 4, A: 255}       // #080404 rail / wells
	ColEmber      = color.NRGBA{R: 255, G: 90, B: 38, A: 255}   // #FF5A26 primary accent
	ColFlame      = color.NRGBA{R: 255, G: 106, B: 26, A: 255}  // #FF6A1A gradient partner
	ColGold       = color.NRGBA{R: 255, G: 197, B: 61, A: 255}  // #FFC53D highlights
	ColText       = color.NRGBA{R: 243, G: 236, B: 234, A: 255} // #F3ECEA primary text
	ColTextMuted  = color.NRGBA{R: 176, G: 152, B: 145, A: 255} // #B09891 secondary text
	ColBorder     = color.NRGBA{R: 66, G: 30, B: 24, A: 255}    // #421E18 separators
	ColBorderSoft = color.NRGBA{R: 44, G: 21, B: 17, A: 255}    // #2C1511 subtle borders
	ColHover      = color.NRGBA{R: 42, G: 18, B: 14, A: 255}    // #2A120E hover fill
	ColInputBg    = color.NRGBA{R: 19, G: 10, B: 9, A: 255}     // #130A09 input fields
	ColDanger     = color.NRGBA{R: 255, G: 59, B: 48, A: 255}   // #FF3B30 errors
	ColSuccess    = color.NRGBA{R: 74, G: 222, B: 128, A: 255}  // #4ADE80 success
)

// fireTheme implements fyne.Theme with the Forge Dark palette.
type fireTheme struct{}

// Theme returns the fire theme singleton.
func Theme() fyne.Theme { return &fireTheme{} }

func (f *fireTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return ColBg
	case theme.ColorNameButton:
		return ColBgRaised
	case theme.ColorNameForeground:
		return ColText
	case theme.ColorNamePrimary:
		return ColEmber
	case theme.ColorNameFocus:
		return color.NRGBA{R: 255, G: 90, B: 38, A: 90}
	case theme.ColorNameHover:
		return ColHover
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 110, G: 92, B: 88, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 26, G: 14, B: 13, A: 255}
	case theme.ColorNameInputBackground:
		return ColInputBg
	case theme.ColorNameInputBorder:
		return ColBorder
	case theme.ColorNameMenuBackground:
		return ColBgRaised
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 5, G: 3, B: 3, A: 220}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 128, G: 106, B: 100, A: 255}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 58, G: 24, B: 18, A: 255}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 90, G: 45, B: 34, A: 200}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 30, G: 15, B: 12, A: 255}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 255, G: 90, B: 38, A: 70}
	case theme.ColorNameSeparator:
		return ColBorder
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 210}
	case theme.ColorNameHeaderBackground:
		return ColBgDeep
	case theme.ColorNameError: // error color lookup (since 2.5 as ColorNameError?)
		return ColDanger
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (f *fireTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

// Icon routes standard theme icon names to the SHEYTAN icon set where a
// matching fire-styled icon exists.
func (f *fireTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	switch name {
	case theme.IconNameCancel:
		return icon("stop")
	case theme.IconNameConfirm:
		return icon("spark")
	case theme.IconNameDelete:
		return icon("warn")
	case theme.IconNameSearch:
		return icon("search")
	case theme.IconNameViewRefresh:
		return icon("refresh")
	case theme.IconNameDocumentCreate:
		return icon("new")
	case theme.IconNameDocumentSave:
		return icon("export")
	case theme.IconNameFolderOpen:
		return icon("folder")
	case theme.IconNameInfo:
		return icon("info")
	case theme.IconNameWarning:
		return icon("warn")
	case theme.IconNameError:
		return icon("warn")
	case theme.IconNameMailSend:
		return icon("send")
	}
	return theme.DefaultTheme().Icon(name)
}

func (f *fireTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 7
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 19
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInputBorder:
		return 1.5
	case theme.SizeNameScrollBar:
		return 9
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}
