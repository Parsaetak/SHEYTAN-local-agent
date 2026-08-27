// v1.0.8 stress scenarios — the AURORA release: the attachment-crash fix
// (native Win32 picker + app-wide panic guards), the Aurora button system,
// the Parsa Tak signature, and the dual-zip distribution.
//
// Hostile-input style throughout: the multiselect buffer parser eats
// garbage and truncated buffers, the filter builder output is checked
// byte-for-byte, and the signature surface is verified end to end.
package cmd

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"

        "github.com/sheytan/local-agent/internal/brand"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/native"
)

// stressV108Surface locks the v1.0.8 release surface: version constant and
// the Parsa Tak signature constants.
func stressV108Surface() error {
        if config.AppVersion != "1.0.8" {
                return fmt.Errorf("AppVersion = %s, want 1.0.8", config.AppVersion)
        }
        if brand.SignedBy != "Parsa Tak" {
                return fmt.Errorf("SignedBy = %q, want \"Parsa Tak\"", brand.SignedBy)
        }
        if brand.SignedByRole == "" {
                return fmt.Errorf("SignedByRole must not be empty")
        }
        // The signature block must name the signer, the product, the licensor
        // and the trademark in one printable block.
        block := brand.SignatureBlock(config.AppVersion)
        for _, want := range []string{
                "Parsa Tak",
                "SHEYTAN-Local-Agent v1.0.8",
                "Parsaetak",
                "https://github.com/Parsaetak",
                "All rights reserved",
                "trademarks of Parsaetak",
        } {
                if !strings.Contains(block, want) {
                        return fmt.Errorf("signature block missing %q:\n%s", want, block)
                }
        }
        // The signature line is the compact form shown in About.
        if line := brand.SignatureLine(); !strings.Contains(line, "Parsa Tak") {
                return fmt.Errorf("SignatureLine missing signer: %q", line)
        }
        return nil
}

// stressV108MultiSelParse feeds the native picker's multi-select buffer
// decoder hostile inputs: single path, multi path, trailing padding,
// empty buffer, all-NULs, and a buffer with no double terminator.
func stressV108MultiSelParse() error {
        // single selection → one full path
        got := native.ParseMultiSelBuf(utf16Buf(`C:\models\gemma.gguf`))
        if len(got) != 1 || got[0] != `C:\models\gemma.gguf` {
                return fmt.Errorf("single = %v", got)
        }

        // multi selection → dir + names joined
        multi := utf16Buf(`C:\docs` + "\x00" + `a.txt` + "\x00" + `b.md` + "\x00" + "\x00")
        got = native.ParseMultiSelBuf(multi)
        if len(got) != 2 || got[0] != `C:\docs\a.txt` || got[1] != `C:\docs\b.md` {
                return fmt.Errorf("multi = %v", got)
        }

        // empty buffer → no paths (caller treats as cancel)
        if got = native.ParseMultiSelBuf(utf16Buf("")); len(got) != 0 {
                return fmt.Errorf("empty = %v, want none", got)
        }
        // all-NUL buffer → no paths
        if got = native.ParseMultiSelBuf(make([]uint16, 64)); len(got) != 0 {
                return fmt.Errorf("all-NUL = %v, want none", got)
        }
        // unterminated garbage → still decodes without panic
        _ = native.ParseMultiSelBuf(utf16Buf(`D:\odd\x00name-without-end`))

        // degenerate: dir-only multiselect (no names) → treated as single path
        dirOnly := utf16Buf(`C:\solo` + "\x00" + "\x00")
        if got = native.ParseMultiSelBuf(dirOnly); len(got) != 1 || got[0] != `C:\solo` {
                return fmt.Errorf("dirOnly = %v", got)
        }
        return nil
}

// stressV108FilterBuild verifies the native picker's filter string
// builder: label\0pattern\0 pairs, double-NUL terminator, and that the
// shape of every pattern the app ships is valid for the OS dialog.
func stressV108FilterBuild() error {
        s := native.BuildFilterString([]native.FileFilter{
                {Label: "Docs", Pattern: "*.txt;*.md"},
                {Label: "All files", Pattern: "*.*"},
        })
        want := "Docs\x00*.txt;*.md\x00All files\x00*.*\x00\x00"
        if s != want {
                return fmt.Errorf("filter = %q, want %q", s, want)
        }
        // a long realistic bucket (the real attach filter) must stay intact:
        // pairs alternate label/pattern and the block ends in a double NUL.
        long := native.BuildFilterString([]native.FileFilter{
                {Label: "Documents", Pattern: strings.Repeat("*.ext;", 40) + "*.zzz"},
        })
        if !strings.HasSuffix(long, "\x00\x00") {
                return fmt.Errorf("long filter missing double-NUL terminator")
        }
        parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(long, "\x00"), "\x00"), "\x00")
        if len(parts)%2 != 0 {
                return fmt.Errorf("filter pairs unbalanced: %d parts", len(parts))
        }
        for i := 0; i < len(parts); i += 2 {
                if parts[i] == "" || parts[i+1] == "" {
                        return fmt.Errorf("empty filter slot at %d", i)
                }
        }
        return nil
}

// stressV108PickerUnavailable proves the fallback contract: on platforms
// without the native dialog (this Linux build), PickFiles reports
// ErrUnavailable instead of crashing — the GUI falls back to the toolkit
// picker on exactly this signal.
func stressV108PickerUnavailable() error {
        res := native.PickFiles("test", nil, "")
        if res.Err == nil {
                // On Windows the dialog would OPEN here (test build is Linux, so
                // this branch is unreachable in CI); treat success as a hard fail
                // because a modal dialog must never appear from the stress suite.
                return fmt.Errorf("native dialog unexpectedly available in headless CI")
        }
        if len(res.Paths) != 0 {
                return fmt.Errorf("unavailable picker returned paths: %v", res.Paths)
        }
        return nil
}

// stressV108Aicontext asserts the v8 instruction file teaches the model
// about the native attachments and the Parsa Tak signature.
func stressV108Aicontext() error {
        data, err := os.ReadFile(filepath.Join("internal", "aicontext", "AI-CONTEXT.md"))
        if err != nil {
                data, err = os.ReadFile("internal/aicontext/AI-CONTEXT.md")
                if err != nil {
                        return fmt.Errorf("AI-CONTEXT.md unreadable: %v", err)
                }
        }
        txt := string(data)
        if !strings.Contains(txt, "sheytan-context-version: 8") {
                return fmt.Errorf("AI-CONTEXT.md marker not v8")
        }
        for _, want := range []string{"Parsa Tak", "multi-select", "mmproj"} {
                if !strings.Contains(txt, want) {
                        return fmt.Errorf("AI-CONTEXT.md missing v1.0.8 topic %q", want)
                }
        }
        return nil
}

// utf16Buf encodes s (which may contain \x00 separators) as UTF-16 units
// with zero padding like the real 64KB dialog buffer.
func utf16Buf(s string) []uint16 {
        runes := []rune(s)
        out := make([]uint16, 0, len(runes)+16)
        for _, r := range runes {
                out = append(out, uint16(r))
        }
        return append(out, make([]uint16, 16)...)
}
