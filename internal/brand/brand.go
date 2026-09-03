// Package brand holds the SHEYTAN™ identity: trademark attribution,
// copyright, and license text. Every surface (GUI, CLI, docs, diagnostics)
// renders these constants so legal notices stay consistent across releases.
//
// Legal structure:
//   - Copyright holder & licensor:  Parsaetak (https://github.com/Parsaetak)
//   - Trademark:                    SHEYTAN™ — a trademark of Parsaetak,
//     used as the product name for this app.
package brand

import "time"

const (
	// Name is the short product name.
	Name = "SHEYTAN"
	// FullName is the complete product name.
	FullName = "SHEYTAN-Local-Agent"
	// Trademark is the brand with the ™ mark, for display surfaces.
	Trademark = "SHEYTAN™"
	// Licensor is the legal entity that owns the copyright and license.
	Licensor = "Parsaetak"
	// LicensorURL is the official licensor contact point.
	LicensorURL = "https://github.com/Parsaetak"
	// TrademarkNotice is the formal trademark attribution line.
	TrademarkNotice = "SHEYTAN and the SHEYTAN logo are trademarks of Parsaetak."
	// LicenseName is the human name of the license.
	LicenseName = "Parsaetak Proprietary License v1.1"

	// SignedBy is the application author/signer — v1.0.8. Every release
	// of SHEYTAN-Local-Agent is authored and signed under this name; it
	// is embedded in the exe version resource (CompanyName), printed in
	// the About dialog, and carried in the SIGNATURE block of each
	// distribution archive.
	SignedBy = "Parsa Tak"
	// SignedByRole is the role line printed under the signature.
	SignedByRole = "Author & Application Signer"
)

// LogoSVG is the full-color flame mark (gradient flame on a deep ember
// disc) used everywhere the brand appears: the app UI, the window icon,
// the splash — and, since v1.0.5, the rendered .exe icon. It lives here so
// the GUI theme and the build-time icon generator can never drift apart.
const LogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 24 24">
<defs>
  <linearGradient id="flame" x1="0" y1="1" x2="0" y2="0">
    <stop offset="0%" stop-color="#C9182B"/>
    <stop offset="45%" stop-color="#FF3B30"/>
    <stop offset="80%" stop-color="#FF6B1A"/>
    <stop offset="100%" stop-color="#FFC53D"/>
  </linearGradient>
  <linearGradient id="flameInner" x1="0" y1="1" x2="0" y2="0">
    <stop offset="0%" stop-color="#FFDD55"/>
    <stop offset="100%" stop-color="#FF8A50"/>
  </linearGradient>
</defs>
<circle cx="12" cy="12" r="11.4" fill="#120808"/>
<circle cx="12" cy="12" r="11.4" fill="none" stroke="#FF3B30" stroke-width=".5" opacity=".8"/>
<path d="M13.5 2.2s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.2 6.6 4 9.7 4 13.1c0 4.42 3.58 8 8 8s8-3.58 8-8c0-5.4-2.59-10.2-6.5-13.33z" fill="url(#flame)"/>
<path d="M12.2 10.4c1.5 1.1 2.6 2.4 2.6 4.2 0 1.9-1.3 3.4-3 3.4-1.1 0-2-.6-2.5-1.5-.2 1.5.8 3.3 2.1 4-.5.1-1 .2-1.6.1-2.4-.3-4.1-2.5-3.8-5 .2-1.6 1.2-2.6 2.2-3.6.9-.9 1.7-1.9 2-3.1.6.7 1.3 1.2 2 1.5z" fill="url(#flameInner)" opacity=".9"/>
</svg>`

// CopyrightYears returns the copyright range "2024–<current year>".
func CopyrightYears() string {
	y := time.Now().Year()
	if y <= 2024 {
		return "2024"
	}
	return "2024–" + itoa(y)
}

// Copyright returns the canonical copyright line, e.g.
// "© 2024–2026 Parsaetak. All rights reserved."
func Copyright() string {
	return "© " + CopyrightYears() + " " + Licensor + ". All rights reserved."
}

// Notice returns the short one-line legal footer used in the UI.
func Notice() string {
	return Trademark + " · " + Copyright()
}

// SignatureLine returns the one-line authorship attribution, e.g.
// "Signed by Parsa Tak — Author & Application Signer".
func SignatureLine() string {
	return "Signed by " + SignedBy + " — " + SignedByRole
}

// SignatureBlock returns the full signature text carried in the About
// dialog and the SIGNATURE file of every distribution archive.
func SignatureBlock(version string) string {
	return "SHEYTAN-Local-Agent v" + version + "\n" +
		"Signed by: " + SignedBy + "\n" +
		"Role:      " + SignedByRole + "\n" +
		"Product:   " + FullName + " (" + Trademark + ")\n" +
		"Licensor:  " + Licensor + " <" + LicensorURL + ">\n" +
		Copyright() + "\n" +
		TrademarkNotice + "\n"
}

// LicenseFooter is the compact attribution block shown in About dialogs.
const LicenseFooter = "SHEYTAN™ is a trademark of Parsaetak.\nLicensed under the Parsaetak Proprietary License v1.1.\n" + LicensorURL

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// LicenseText is the full end-user license agreement shipped with the app.
const LicenseText = `
SHEYTAN™ END-USER LICENSE AGREEMENT
===================================
` + "Version 1.1 · Last updated: 2026-08-26" + `

` + "Copyright © 2024–2026 Parsaetak (https://github.com/Parsaetak). All rights reserved." + `

IMPORTANT — READ CAREFULLY. By installing, copying, or otherwise using
SHEYTAN-Local-Agent ("the Software") you agree to be bound by the terms of
this agreement. If you do not agree, do not install or use the Software.

1. GRANT OF LICENSE
   Subject to the terms below, Parsaetak ("the Licensor") grants you a
   personal, non-exclusive, non-transferable, revocable license to install
   and run the Software on computers you own or control, for any lawful
   personal or commercial purpose.

2. INTELLECTUAL PROPERTY
   The Software and all of its source code, binaries, documentation, icons,
   and design are and remain the exclusive property of Parsaetak and its
   contributors. No ownership rights are transferred to you by this
   license.

3. TRADEMARK
   "SHEYTAN", "SHEYTAN-Local-Agent", and the SHEYTAN logo are trademarks of
   Parsaetak. You may not use the marks (or confusingly similar marks) to
   name or promote products, forks, or derivative works without prior
   written permission. Referring to the Software by its name for the
   purpose of description, review, or interoperability is permitted.

4. DISTRIBUTION
   You may NOT redistribute, sublicense, sell, rent, lease, or host the
   Software (in whole or in part, original or modified) without prior
   written permission. Sharing an unmodified official release archive for
   non-commercial personal use is permitted, provided all files — including
   this license — remain intact.

5. DERIVATIVE WORKS
   You may modify the Software for your own personal use. You may NOT
   distribute modified versions, rebranded builds, or extractions of the
   source code without prior written permission.

6. LOCAL-FIRST PRIVACY
   The Software is designed to run inference and tool execution locally on
   your machine, with all data stored inside the application folder. When
   the optional remote provider mode is enabled, prompts you submit are
   sent to the third-party endpoint you configure; the Licensor is not
   responsible for how third parties handle that data. Local logs (tool
   calls, LLM calls, crashes) stay on your disk until you choose to export
   them.

7. ACCEPTABLE USE
   You agree to use the Software only in compliance with applicable law.
   You are solely responsible for commands the agent executes on your
   behalf, including any file, shell, browser, git, or network operations.

8. DISCLAIMER OF WARRANTY
   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
   OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
   MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND NONINFRINGEMENT.
   IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
   CLAIM, DAMAGES, OR OTHER LIABILITY ARISING FROM, OUT OF, OR IN
   CONNECTION WITH THE SOFTWARE OR ITS USE.

9. TERMINATION
   This license terminates automatically if you breach any term. Upon
   termination you must stop using the Software and delete all copies.

10. CHANGES
   The Licensor may revise this agreement for future releases. Continued
   use after an update constitutes acceptance of the revised terms.

Contact for licensing and trademark inquiries:
  Parsaetak — https://github.com/Parsaetak
`
