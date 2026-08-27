// Package ui — professional fire-styled SVG icon set for every part of
// SHEYTAN-Local-Agent. All icons share one design language:
//   - 24×24 viewBox, 1.7px rounded strokes, ember-orange accents
//   - flat geometry that stays crisp from 16px (rail) to 512px (splash)
//
// Icons are generated with two color variants (bright = active/brand,
// muted = idle) so the navigation rail can animate state without
// re-rasterizing images.
package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/sheytan/local-agent/internal/brand"
)

// Icon colors — matched to theme.go.
const (
	emberBright = "#FF5A26" // active rail / brand accents
	emberMuted  = "#7A5648" // idle rail icons
	emberSoft   = "#FF8A50" // secondary strokes
)

// iconSVG wraps an icon body in a 24×24 SVG document with the given color.
// v1.0.8: stroke 1.9 — the confident weight the 2025/2026 AI platform
// icon sets (Lucide/Phosphor school) draw at on a 24-grid.
func iconSVG(body, color string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="%s" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">%s</svg>`, color, body)
}

func resource(name, svg string) *fyne.StaticResource {
	return fyne.NewStaticResource(name+".svg", []byte(svg))
}

// recolor swaps the bright variant to another color.
func recolor(svg, color string) string {
	return strings.Replace(svg, fmt.Sprintf(`stroke="%s"`, emberBright), fmt.Sprintf(`stroke="%s"`, color), 1)
}

// --- icon bodies (pure geometry, no <text> — Fyne's SVG rasterizer skips text) ---

const bodyFlame = `<path d="M13.5 2.2s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.2 6.6 4 9.7 4 13.1c0 4.42 3.58 8 8 8s8-3.58 8-8c0-5.4-2.59-10.2-6.5-13.33z" fill="currentColor"/>`

const bodyChat = `<path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v8a2.5 2.5 0 0 1-2.5 2.5H10l-4.2 3.4c-.5.4-1.3 0-1.3-.7V6.5z"/><path d="M8 9.5h8M8 12.7h5.5" opacity=".55"/>`

const bodyData = `<path d="M4.5 20h15"/><path d="M7.5 20v-6"/><path d="M12 20v-9.5"/><path d="M16.5 20V7"/><path d="M5 7.5l4.4-3.2 4 2.8 5.1-4.3" stroke-dasharray="0" opacity=".75"/>`

const bodyMemory = `<ellipse cx="12" cy="6.2" rx="6.8" ry="2.8"/><path d="M5.2 6.2v11.6c0 1.55 3.04 2.8 6.8 2.8s6.8-1.25 6.8-2.8V6.2"/><path d="M5.2 12c0 1.55 3.04 2.8 6.8 2.8s6.8-1.25 6.8-2.8" opacity=".6"/>`

const bodyLogs = `<rect x="3.5" y="4.5" width="17" height="15" rx="2.2"/><path d="M7 10l2.8 2.2L7 14.4"/><path d="M11.6 14.6h5" opacity=".6"/>`

const bodySessions = `<path d="M4.6 12a7.4 7.4 0 1 1 2.2 5.2"/><path d="M4.4 13.6l.2 3 3-.4"/><path d="M12 8.4V12l2.6 1.7"/>`

const bodySettings = `<path d="M4 7.3h16M4 12h16M4 16.7h16" opacity=".35"/><circle cx="9" cy="7.3" r="2.1" fill="#0D0707"/><circle cx="15.2" cy="12" r="2.1" fill="#0D0707"/><circle cx="7.4" cy="16.7" r="2.1" fill="#0D0707"/>`

// bodySend — v1.0.8: the ChatGPT-school upward arrow. On a 24 grid it is
// the single most recognizable "send" glyph in modern AI products; the old
// paper-plane read busy at 17px inside the send disc.
const bodySend = `<path d="M12 19.2V4.8"/><path d="m5.6 11.2 6.4-6.4 6.4 6.4"/>`

const bodyStop = `<circle cx="12" cy="12" r="8.2"/><rect x="8.8" y="8.8" width="6.4" height="6.4" rx="1.2" fill="currentColor" stroke="none"/>`

const bodyNew = `<circle cx="12" cy="12" r="8.2"/><path d="M12 8.4v7.2M8.4 12h7.2"/>`

const bodySearch = `<circle cx="10.8" cy="10.8" r="5.6"/><path d="M15 15l4.4 4.4"/>`

const bodyBrowser = `<circle cx="12" cy="12" r="8.2"/><path d="M3.8 12h16.4"/><ellipse cx="12" cy="12" rx="3.6" ry="8.2" opacity=".6"/><path d="M5.4 7.4c3.4 1.7 9.8 1.7 13.2 0M5.4 16.6c3.4-1.7 9.8-1.7 13.2 0" opacity=".6"/>`

const bodyGit = `<circle cx="7" cy="6" r="2.2"/><circle cx="7" cy="18" r="2.2"/><circle cx="17" cy="12" r="2.2"/><path d="M7 8.2v7.6"/><path d="M17 9.8c0-2.4-2-3.6-4.2-3.6H9.2"/><path d="M17 14.2c0 2.4-2 3.6-4.2 3.6H9.2" opacity=".0"/>`

const bodyShell = `<rect x="3.5" y="4.5" width="17" height="15" rx="2.2"/><path d="M7 9.5l3 2.5-3 2.5"/><path d="M12.4 14.6h4.6" opacity=".6"/>`

const bodyFiles = `<path d="M3.5 7.2c0-1 .8-1.7 1.7-1.7h4.2l1.9 2.2h7.5c1 0 1.7.8 1.7 1.7v8.6c0 1-.8 1.7-1.7 1.7H5.2c-1 0-1.7-.8-1.7-1.7V7.2z"/>`

const bodyProvider = `<path d="M7.4 18.5a4.2 4.2 0 0 1-.5-8.4 5.6 5.6 0 0 1 10.9-.9 3.6 3.6 0 0 1 .6 7.1"/><path d="M12.8 9.2 9.6 13.4h4.2l-3.2 4.2" stroke-width="1.9"/>`

const bodyModel = `<rect x="7" y="7" width="10" height="10" rx="2"/><rect x="10.2" y="10.2" width="3.6" height="3.6" rx=".8" opacity=".6"/><path d="M9.4 7V4.6M14.6 7V4.6M9.4 19.4V17M14.6 19.4V17M7 9.4H4.6M7 14.6H4.6M19.4 9.4H17M19.4 14.6H17"/>`

const bodySandbox = `<path d="M12 3.4 5 6v5.4c0 4.3 2.9 7.4 7 9.2 4.1-1.8 7-4.9 7-9.2V6l-7-2.6z"/><path d="M9.2 12.2l2 2.2 3.6-4.2"/>`

const bodySystem = `<circle cx="12" cy="12" r="8.4"/><path d="M12 12 15.6 8.4"/><circle cx="12" cy="12" r="1.3" fill="currentColor" stroke="none"/><path d="M12 3.6v1.8M12 18.6v1.8M3.6 12h1.8M18.6 12h1.8" opacity=".5"/>`

const bodyLicense = `<path d="M6.4 3.6h9.2a2 2 0 0 1 2 2v13.6a1.6 1.6 0 0 1-1.6 1.6H6.4"/><path d="M6.4 3.6a1.6 1.6 0 0 0 0 3.2h10.8"/><circle cx="13.4" cy="13.6" r="2.5"/><path d="M8.8 9.4h4" opacity=".6"/>`

const bodyRefresh = `<path d="M19.4 12a7.4 7.4 0 1 1-2.7-5.7"/><path d="M19.6 4.6v4.2h-4.2"/>`

const bodyExport = `<path d="M12 4.4v10"/><path d="M8.4 11 12 14.6 15.6 11"/><path d="M4.6 16.6v1.6a1.8 1.8 0 0 0 1.8 1.8h11.2a1.8 1.8 0 0 0 1.8-1.8v-1.6"/>`

const bodyFolderOpen = `<path d="M3.5 7c0-1 .8-1.7 1.7-1.7h4l1.9 2.1h7.4c1 0 1.7.8 1.7 1.7v.8"/><path d="M3.5 8.8h17.4l-2 8c-.2.9-1 1.5-1.9 1.5H5.6c-1 0-1.8-.8-1.9-1.8L3.5 8.8z"/>`

const bodyAgent = `<path d="M12 3.2 13.9 9l5.9.2-4.7 3.7 1.7 5.7-4.8-3.2-4.8 3.2 1.7-5.7L4.2 9.2 10.1 9z"/>`

const bodyUser = `<circle cx="12" cy="8.2" r="3.7"/><path d="M5.2 20c.6-3.6 3.4-5.6 6.8-5.6s6.2 2 6.8 5.6"/>`

const bodyWarn = `<path d="M12 4 3.4 18.6h17.2L12 4z"/><path d="M12 10.2v4"/><circle cx="12" cy="16.6" r=".9" fill="currentColor" stroke="none"/>`

const bodyInfo = `<circle cx="12" cy="12" r="8.4"/><path d="M12 11v5"/><circle cx="12" cy="8" r=".9" fill="currentColor" stroke="none"/>`

// bodyTools — v1.0.8: the clean Lucide wrench — a single confident
// silhouette (the v1.0.7 wrench was skewed by a transform).
const bodyTools = `<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>`

const bodyContext = `<path d="M12 3.6 3.6 7.8 12 12l8.4-4.2L12 3.6z"/><path d="M3.6 12.2 12 16.4l8.4-4.2" opacity=".6"/><path d="M3.6 16.4 12 20.6l8.4-4.2" opacity=".35"/>`

const bodySpark = `<path d="M12 4v4M12 16v4M4 12h4M16 12h4"/><path d="M12 8.6 13.4 12 12 15.4 10.6 12z" fill="currentColor" stroke="none"/>`

const bodyMenu = `<circle cx="12" cy="5.4" r="1.3" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.3" fill="currentColor" stroke="none"/><circle cx="12" cy="18.6" r="1.3" fill="currentColor" stroke="none"/>`

const bodyChevron = `<path d="M6.4 9.4 12 15l5.6-5.6"/>`

const bodyCheck = `<path d="M5.2 12.8l4.2 4.2 9.4-10"/>`

const bodyUpdate = `<path d="M12 3.6v5"/><path d="M12 3.6 9.4 6.2M12 3.6l2.6 2.6" opacity=".0"/><path d="M6.1 6.5a7.8 7.8 0 1 0 11.9 1.9"/><path d="M18.9 3.8v4.6h-4.6"/>`

const bodyPlus = `<path d="M12 5v14M5 12h14"/>`

const bodyPanel = `<rect x="3.6" y="4.6" width="16.8" height="14.8" rx="2.2"/><path d="M14.6 4.8v14.4"/><path d="M6.4 8.6h5M6.4 12h5M6.4 15.4h3.4" opacity=".6"/>`

const bodyEye = `<path d="M2.8 12S6 5.8 12 5.8 21.2 12 21.2 12 18 18.2 12 18.2 2.8 12 2.8 12z"/><circle cx="12" cy="12" r="2.6"/>`

// bodyAttach — v1.0.8: the modern slim paperclip (Lucide geometry) —
// continuous single stroke, optically balanced.
const bodyAttach = `<path d="m21.15 11.15-9.02-9.02a5.25 5.25 0 0 0-7.42 7.42l9.55 9.55a3.5 3.5 0 0 0 4.95-4.95l-9.55-9.55a1.75 1.75 0 0 0-2.47 2.47l9.38 9.38"/>`

const bodyBrain = `<path d="M9.5 4.2a2.8 2.8 0 0 0-2.8 2.8c-1.6.3-2.7 1.7-2.7 3.3 0 1 .4 1.9 1.1 2.5-.4.6-.6 1.3-.6 2 0 1.9 1.5 3.5 3.4 3.6.3 1.5 1.7 2.6 3.3 2.6.9 0 1.8-.4 2.3-1V6.7a2.5 2.5 0 0 0-4-2.5z"/><path d="M14.5 4.2a2.8 2.8 0 0 1 2.8 2.8c1.6.3 2.7 1.7 2.7 3.3 0 1-.4 1.9-1.1 2.5.4.6.6 1.3.6 2 0 1.9-1.5 3.5-3.4 3.6-.3 1.5-1.7 2.6-3.3 2.6-.9 0-1.8-.4-2.3-1" opacity=".55"/><path d="M9.7 9.4v.01M12.9 12.6v.01M9.7 14.6v.01" stroke-width="2.2"/>`

const bodyClose = `<path d="M6.4 6.4l11.2 11.2M17.6 6.4L6.4 17.6"/>`

// v1.0.3 file-type icons (attachment tiles): one recognizable glyph per
// attachment family so staged files read at a glance.

const bodyImage = `<rect x="3.4" y="5" width="17.2" height="14" rx="2.2"/><circle cx="8.6" cy="10" r="1.6"/><path d="M4.2 17.2l4.6-4.4 3.2 3 3-2.8 4.6 4.2"/>`

const bodyAudio = `<path d="M9 18.2V6.4l9-1.8v11.6"/><circle cx="6.6" cy="18.2" r="2.4"/><circle cx="15.6" cy="16.2" r="2.4"/>`

const bodyVideo = `<rect x="3.2" y="5.4" width="12.4" height="13.2" rx="2.2"/><path d="M15.6 10.4l5.2-3v8.4l-5.2-3"/>`

const bodyArchive = `<rect x="4" y="4.2" width="16" height="5.6" rx="1.4"/><path d="M5.6 9.8v8.4c0 1 .8 1.6 1.6 1.6h9.6c.9 0 1.6-.7 1.6-1.6V9.8"/><path d="M10 13.6h4" opacity=".6"/>`

const bodyCode = `<path d="M8.6 8.4 4.4 12.4l4.2 4"/><path d="M15.4 8.4l4.2 4-4.2 4"/><path d="M13.2 5.6l-2.4 12.8"/>`

const bodyDoc = `<path d="M6 3.8h8l4.4 4.4v12c0 .9-.7 1.6-1.6 1.6H6c-.9 0-1.6-.7-1.6-1.6V5.4c0-.9.7-1.6 1.6-1.6z"/><path d="M13.6 3.8v4.8h4.6"/><path d="M8 12.6h8M8 15.8h5.4" opacity=".6"/>`

const bodyGguf = `<rect x="5" y="7.6" width="14" height="12" rx="2.2"/><path d="M5 11h14" opacity=".5"/><circle cx="8.4" cy="14.6" r="1.1" fill="currentColor" stroke="none"/><circle cx="12" cy="14.6" r="1.1" fill="currentColor" stroke="none"/><circle cx="15.6" cy="14.6" r="1.1" fill="currentColor" stroke="none"/><path d="M8.4 7.6V5.2h7.2v2.4"/>`

const bodyEngine = `<path d="M12 3.6v3M12 17.4v3M3.6 12h3M17.4 12h3"/><circle cx="12" cy="12" r="5.2"/><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none"/><path d="M12 6.8c2 0 3.2 1.4 3.2 2.6" opacity=".6"/>`

// v1.0.4 artifact/file action icons.

const bodyCopy = `<rect x="8.6" y="8.6" width="11.8" height="11.8" rx="2"/><path d="M15.4 8.6V6.2c0-1-.8-1.8-1.8-1.8H6.2c-1 0-1.8.8-1.8 1.8v7.4c0 1 .8 1.8 1.8 1.8h2.4"/>`

const bodyOpen = `<path d="M14 4.4h5.6V10"/><path d="M19.6 4.4 11 13"/><path d="M9.4 6.2H6.2c-1 0-1.8.8-1.8 1.8v9.8c0 1 .8 1.8 1.8 1.8H16c1 0 1.8-.8 1.8-1.8v-3.2"/>`

const bodyBolt = `<path d="M13.6 2.6 5.2 13.4h5l-1.8 8 8.4-11h-5.2z"/>`

const bodyDelete = `<path d="M4.6 7.4h14.8"/><path d="M9.4 7.4V5.2c0-.7.5-1.2 1.2-1.2h2.8c.7 0 1.2.5 1.2 1.2v2.2"/><path d="M6.4 7.4l1 11.2c.1 1 .9 1.8 1.9 1.8h5.4c1 0 1.8-.8 1.9-1.8l1-11.2"/><path d="M10.2 11v6M13.8 11v6" opacity=".6"/>`

// v1.0.6 icons — vision, terminal, resources, feedback, palette.

// bodyCamera — v1.0.8: modern rounded camera — softer body corners,
// larger optically-centered lens (screenshot capture control).
const bodyCamera = `<path d="M4.4 8.6h2.4l1.5-2.3h7.4l1.5 2.3h2.4a1.6 1.6 0 0 1 1.6 1.6v7.2a1.6 1.6 0 0 1-1.6 1.6H4.4a1.6 1.6 0 0 1-1.6-1.6v-7.2a1.6 1.6 0 0 1 1.6-1.6z"/><circle cx="12" cy="13.7" r="3.3"/>`

const bodyTerminal = `<rect x="3.2" y="4.6" width="17.6" height="14.8" rx="2.2"/><path d="M6.8 10l2.8 2.6-2.8 2.6"/><path d="M11.6 15.4h5.6"/>`

const bodyGauge = `<path d="M4.4 17.8a8.6 8.6 0 1 1 15.2 0"/><path d="M12 17.4l4-5.2"/><circle cx="12" cy="17.8" r="1.4" fill="currentColor" stroke="none"/><path d="M6.2 13.2l1.2.8M17.8 13.2l-1.2.8M12 8.6v1.6" opacity=".6"/>`

const bodyThumbUp = `<path d="M7.4 10.6V19H5.6c-1 0-1.8-.8-1.8-1.8v-4.8c0-1 .8-1.8 1.8-1.8h1.8z"/><path d="M7.4 10.6l3.4-5.6c1.4 0 2.4 1.2 2.2 2.6l-.4 3h4.6c1.2 0 2.2 1 2 2.2l-.8 4.4c-.2 1-1 1.8-2 1.8H7.4"/>`

const bodyThumbDown = `<path d="M7.4 13.4V5h1.8c1 0 1.8.8 1.8 1.8v4.8c0 1-.8 1.8-1.8 1.8H7.4z"/><path d="M7.4 13.4l3.4 5.6c1.4 0 2.4-1.2 2.2-2.6l-.4-3h4.6c1.2 0 2.2-1 2-2.2l-.8-4.4c-.2-1-1-1.8-2-1.8H7.4"/>`

const bodyCommand = `<path d="M9 9V6.6A2.4 2.4 0 1 0 6.6 9H9zm0 0v6m0-6h6m-6 6H6.6A2.4 2.4 0 1 0 9 17.4V15zm6-6V6.6A2.4 2.4 0 1 1 17.4 9H15zm0 0v6m0 0h2.4a2.4 2.4 0 1 1-2.4 2.4V15z"/>`

// v1.0.7 icons — continuum (chapter rollover, context meter).

const bodyLayers = `<path d="M12 3.4l8.2 4.2L12 11.8 3.8 7.6 12 3.4z"/><path d="M5.9 11.4L3.8 12.4l8.2 4.2 8.2-4.2-2.1-1" opacity=".75"/><path d="M5.9 15.6l-2.1 1 8.2 4.2 8.2-4.2-2.1-1" opacity=".5"/>`

// --- icon library ---

// Icons holds every named icon used by the app (bright variants).
var Icons = map[string]*fyne.StaticResource{}

// IconsMuted holds the idle/muted color variants.
var IconsMuted = map[string]*fyne.StaticResource{}

// iconBodies exposes the raw SVG bodies (for variant colors like the white
// send arrow).
var iconBodies map[string]string

func init() {
	bodies := map[string]string{
		"chat":     bodyChat,
		"data":     bodyData,
		"memory":   bodyMemory,
		"logs":     bodyLogs,
		"sessions": bodySessions,
		"settings": bodySettings,
		"send":     bodySend,
		"stop":     bodyStop,
		"new":      bodyNew,
		"search":   bodySearch,
		"browser":  bodyBrowser,
		"git":      bodyGit,
		"shell":    bodyShell,
		"files":    bodyFiles,
		"provider": bodyProvider,
		"model":    bodyModel,
		"sandbox":  bodySandbox,
		"system":   bodySystem,
		"license":  bodyLicense,
		"refresh":  bodyRefresh,
		"export":   bodyExport,
		"folder":   bodyFolderOpen,
		"agent":    bodyAgent,
		"user":     bodyUser,
		"warn":     bodyWarn,
		"info":     bodyInfo,
		"tools":    bodyTools,
		"context":  bodyContext,
		"spark":    bodySpark,
		"menu":     bodyMenu,
		"chevron":  bodyChevron,
		"check":    bodyCheck,
		"update":   bodyUpdate,
		"plus":     bodyPlus,
		"panel":    bodyPanel,
		"eye":      bodyEye,
		"attach":   bodyAttach,
		"brain":    bodyBrain,
		"close":    bodyClose,
		// v1.0.3 file-type icons (attachment tiles)
		"image":   bodyImage,
		"audio":   bodyAudio,
		"video":   bodyVideo,
		"archive": bodyArchive,
		"code":    bodyCode,
		"doc":     bodyDoc,
		"gguf":    bodyGguf,
		"engine":  bodyEngine,
		"copy":    bodyCopy,
		"open":    bodyOpen,
		"bolt":    bodyBolt,
		"delete":  bodyDelete,
		// v1.0.6
		"camera":    bodyCamera,
		"terminal":  bodyTerminal,
		"layers":    bodyLayers,
		"gauge":     bodyGauge,
		"thumbUp":   bodyThumbUp,
		"thumbDown": bodyThumbDown,
		"command":   bodyCommand,
	}
	iconBodies = bodies
	for name, body := range bodies {
		Icons[name] = resource("sheytan-"+name, iconSVG(body, emberBright))
		IconsMuted[name] = resource("sheytan-"+name+"-muted", recolor(iconSVG(body, emberBright), emberMuted))
	}
}

// icon returns the bright icon resource (nil-safe fallback: nil).
func icon(name string) fyne.Resource {
	if r, ok := Icons[name]; ok {
		return r
	}
	return nil
}

// iconMuted returns the muted variant.
func iconMuted(name string) fyne.Resource {
	if r, ok := IconsMuted[name]; ok {
		return r
	}
	return nil
}

// iconForFile maps a file name to its attachment-family icon name
// (v1.0.3 attachment tiles).
func iconForFile(name string) string {
	switch strings.ToLower(filepathExt(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico", ".tiff":
		return "image"
	case ".mp3", ".wav", ".ogg", ".flac", ".m4a", ".aac", ".opus":
		return "audio"
	case ".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv", ".wmv":
		return "video"
	case ".zip", ".tar", ".gz", ".7z", ".rar", ".bz2", ".xz", ".zst":
		return "archive"
	case ".gguf":
		return "gguf"
	case ".go", ".py", ".rs", ".java", ".kt", ".js", ".mjs", ".cjs", ".ts", ".tsx",
		".jsx", ".vue", ".svelte", ".c", ".h", ".cpp", ".cc", ".hpp", ".cs", ".rb",
		".php", ".pl", ".lua", ".r", ".m", ".sql", ".sh", ".bash", ".zsh", ".fish",
		".ps1", ".psm1", ".bat", ".cmd", ".html", ".htm", ".css", ".scss", ".json",
		".jsonl", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env", ".xml",
		".diff", ".patch", ".log":
		return "code"
	case ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".pptx", ".ppt", ".odt", ".rtf":
		return "doc"
	case ".txt", ".md", ".markdown", ".mdx", ".csv", ".tsv", ".srt", ".vtt":
		return "doc"
	default:
		return "files"
	}
}

// filepathExt is filepath.Ext without importing path/filepath here.
func filepathExt(name string) string {
	for i := len(name) - 1; i >= 0 && !osIsPathSep(name[i]); i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

func osIsPathSep(b byte) bool { return b == '/' || b == '\\' }

// logoSVG returns the full-color flame logo (gradient + glow) at any size.
// v1.0.5: the mark itself lives in internal/brand — one source of truth
// shared with the .exe icon generator.
func logoSVG() string {
	return brand.LogoSVG
}

// Logo is the app logo resource (used for window icon + splash).
var Logo = resource("sheytan-logo", logoSVG())

// LogoDim is a lower-key flame for idle states.
var LogoDim = resource("sheytan-logo-dim", strings.ReplaceAll(logoSVG(), `stop-color="#FF`, `stop-color="#7`))
