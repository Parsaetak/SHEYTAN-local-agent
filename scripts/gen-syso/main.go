// gen-syso — v1.0.6: builds the Windows resource object (rsrc_windows_amd64.syso)
// that `go build` embeds into sheytan-local-agent.exe automatically.
//
// It carries three things the exe never had before:
//
//  1. THE APP ICON — the brand flame (brand.LogoSVG) rendered at 512px and
//     packed into a multi-size Windows icon (256/128/64/48/32/16). Explorer,
//     the taskbar, and window listings now show SHEYTAN with its own face
//     instead of the generic Go placeholder.
//
//  2. VERSION INFO — FileVersion / ProductName / Company / Copyright, all
//     derived from internal/brand + internal/config so they can never drift
//     from what the app itself reports.
//
//  3. A DPI-AWARE MANIFEST — PerMonitorV2 + system fallback + Common
//     Controls v6 + long-path awareness. This is the second half of the
//     "panels are so small" fix: before the manifest the process was
//     DPI-unaware, so Windows reported 96 DPI to Fyne no matter what the
//     display scaling really was — on a 125-150% laptop the WHOLE interface
//     rendered at 1x and every dialog, tab and control looked miniaturized.
//     With the manifest, GLFW/Fyne see the true scale and render crisp at
//     native size.
//
// Run from the module root:  go run ./scripts/gen-syso
package main

import (
        "fmt"
        "image"
        "image/png"
        "os"
        "strconv"
        "strings"

        "github.com/fyne-io/oksvg"
        "github.com/srwiley/rasterx"
        "github.com/tc-hib/winres"
        "github.com/tc-hib/winres/version"

        "github.com/sheytan/local-agent/internal/brand"
        "github.com/sheytan/local-agent/internal/config"
)

func main() {
        // 1) Render the brand flame at 512x512 (rasterized from the same SVG the
        //    UI uses — one mark, everywhere).
        img, err := renderLogo(512)
        if err != nil {
                fatal("render logo: %v", err)
        }

        // Preview artifact so the icon can be eyeballed without a Windows box.
        if err := os.MkdirAll("build", 0o755); err != nil {
                fatal("mkdir build: %v", err)
        }
        if f, err := os.Create("build/icon-preview.png"); err == nil {
                _ = png.Encode(f, img)
                _ = f.Close()
        }

        // 2) Pack the multi-size icon (winres resizes with a high-quality filter
        //    internally and writes valid ICO-format resource entries).
        icon, err := winres.NewIconFromResizedImage(img, []int{256, 128, 64, 48, 32, 16})
        if err != nil {
                fatal("build icon: %v", err)
        }

        rs := &winres.ResourceSet{}
        if err := rs.SetIcon(winres.ID(1), icon); err != nil {
                fatal("set icon: %v", err)
        }

        // 3) Version info — derived from the app's own constants.
        ver := versionNumbers(config.AppVersion)
        vi := version.Info{
                FileVersion:    ver,
                ProductVersion: ver,
        }
        str := func(key, val string) {
                if err := vi.Set(version.LangDefault, key, val); err != nil {
                        fatal("version info %s: %v", key, err)
                }
        }
        str(version.FileDescription, "SHEYTAN\u2122 Local-Agent — local AI agent for Windows")
        str(version.ProductName, config.AppName)
        // v1.0.8 — the application is signed under the author's name: the exe's
        // CompanyName field carries the signer (Parsa Tak); the legal licensor
        // stays Parsaetak in the copyright line. Signature line is also
        // embedded so "right-click → Properties → Details" shows the signer.
        str(version.CompanyName, brand.SignedBy)
        str(version.LegalCopyright, brand.Copyright()+" "+brand.TrademarkNotice+" Signed by "+brand.SignedBy+".")
        str(version.LegalTrademarks, brand.TrademarkNotice)
        str(version.Comments, brand.SignatureLine())
        str(version.OriginalFilename, "sheytan-local-agent.exe")
        str(version.InternalName, "sheytan-local-agent")
        str(version.FileVersion, versionString(config.AppVersion))
        str(version.ProductVersion, config.AppVersion)
        rs.SetVersionInfo(vi)

        // 4) Manifest: DPI awareness + common controls + long paths.
        rs.SetManifest(winres.AppManifest{
                Description:         config.AppName,
                DPIAwareness:        winres.DPIPerMonitorV2,
                UseCommonControlsV6: true,
                LongPathAware:       true,
                ExecutionLevel:      winres.AsInvoker,
                Compatibility:       winres.Win10AndAbove,
        })

        // 5) Emit the .syso next to main.go — the Go toolchain links it into
        //    windows/amd64 builds automatically (other platforms ignore it).
        out, err := os.Create("rsrc_windows_amd64.syso")
        if err != nil {
                fatal("create syso: %v", err)
        }
        defer out.Close()
        if err := rs.WriteObject(out, winres.ArchAMD64); err != nil {
                fatal("write syso: %v", err)
        }

        fmt.Printf("rsrc_windows_amd64.syso written — icon + version %s + DPI-aware manifest embedded\n",
                config.AppVersion)
}

// renderLogo rasterizes the brand SVG at size x size pixels.
func renderLogo(size int) (image.Image, error) {
        icon, err := oksvg.ReadIconStream(strings.NewReader(brand.LogoSVG))
        if err != nil {
                return nil, err
        }
        icon.SetTarget(0, 0, float64(size), float64(size))
        img := image.NewRGBA(image.Rect(0, 0, size, size))
        scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
        raster := rasterx.NewDasher(size, size, scanner)
        icon.Draw(raster, 1.0)
        return img, nil
}

// versionNumbers turns "1.0.6" into [4]uint16{1, 0, 6, 0}.
func versionNumbers(v string) [4]uint16 {
        var out [4]uint16
        for i, part := range strings.SplitN(strings.TrimSpace(v), ".", 4) {
                if i >= 4 {
                        break
                }
                if n, err := strconv.ParseUint(part, 10, 16); err == nil {
                        out[i] = uint16(n)
                }
        }
        return out
}

// versionString renders "1.0.6" as the canonical "1.0.6.0".
func versionString(v string) string {
        parts := strings.SplitN(strings.TrimSpace(v), ".", 4)
        for len(parts) < 4 {
                parts = append(parts, "0")
        }
        return strings.Join(parts, ".")
}

func fatal(format string, args ...interface{}) {
        fmt.Fprintf(os.Stderr, "gen-syso: "+format+"\n", args...)
        os.Exit(1)
}
