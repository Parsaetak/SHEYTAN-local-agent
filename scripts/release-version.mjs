#!/usr/bin/env node
/**
 * release-version.mjs — Single-source-of-truth release metadata gate for
 * SHEYTAN-Local-Agent (Zeta line).
 *
 * The canonical version lives in package.json ("version", e.g. "1.1.3-zeta").
 * Every other release surface is derived from it and must stay in sync:
 *
 *   - internal/config/config.go            ->  AppVersion  = "1.1.3"        (base version)
 *   - build/config.yml                     ->  productVersion: "1.1.3-zeta"  (full version)
 *   - SIGNATURE                            ->  SHEYTAN-Local-Agent v1.1.3     (base version)
 *   - .github/workflows/build-desktop.yml  ->  APP_VERSION: "1.1.3"           (base version)
 *
 * Usage:
 *   node scripts/release-version.mjs            # sync mode (default): repair drift in place
 *   node scripts/release-version.mjs --check    # verify only: exit 1 on any drift
 *
 * Designed for CI: zero dependencies, CRLF-safe, never rewrites a file that
 * is already correct, and emits GitHub Actions ::error:: annotations in
 * check mode so drift is visible directly on the run summary.
 */

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);

const CHECK_ONLY = process.argv.includes("--check");

const targets = [
  {
    label: "internal/config/config.go",
    file: "internal/config/config.go",
    // gofmt-aligned const, e.g. `      AppVersion  = "1.1.3"`
    pattern: /((?:^|\r?\n)[ \t]*AppVersion[ \t]*=[ \t]*")([^"]*)(")/,
    expected: (base) => base,
    describe: (v) => `AppVersion = "${v}"`,
  },
  {
    label: "build/config.yml",
    file: join("build", "config.yml"),
    pattern: /((?:^|\r?\n)[ \t]*productVersion:[ \t]*")([^"]*)(")/,
    expected: (base, full) => full,
    describe: (v) => `productVersion: "${v}"`,
  },
  {
    label: "SIGNATURE",
    file: "SIGNATURE",
    // First line only: "SHEYTAN-Local-Agent v1.1.3"
    pattern: /(^[ \t]*SHEYTAN-Local-Agent[ \t]+v)([^\r\n]*)/,
    multiline: true,
    // group(2) holds only the version token after the "SHEYTAN-Local-Agent v"
    // prefix, so the splice value is the bare base version.
    expected: (base) => base,
    describe: (v) => `first line = "SHEYTAN-Local-Agent v${v}"`,
  },
  {
    label: ".github/workflows/build-desktop.yml",
    file: join(".github", "workflows", "build-desktop.yml"),
    pattern: /((?:^|\r?\n)[ \t]*APP_VERSION:[ \t]*")([^"]*)(")/,
    expected: (base) => base,
    describe: (v) => `APP_VERSION: "${v}"`,
  },
];

function fail(message) {
  if (CHECK_ONLY) {
    // GitHub Actions annotation so the drift shows up on the run summary.
    console.error(`::error::${message}`);
  }
  console.error(`[sheytan-release] ERROR: ${message}`);
  process.exit(1);
}

// ---------------------------------------------------------------------------
// 1. Read the canonical version from package.json.
// ---------------------------------------------------------------------------

let packageVersion;
try {
  const pkg = JSON.parse(readFileSync(join(repoRoot, "package.json"), "utf8"));
  packageVersion = pkg.version;
} catch (error) {
  fail(`unable to read package.json: ${error.message}`);
}

if (typeof packageVersion !== "string" || packageVersion.length === 0) {
  fail("package.json has no usable \"version\" field.");
}

const suffixMatch = packageVersion.match(/^(\d+\.\d+\.\d+)(?:-(.+))?$/);

if (!suffixMatch) {
  fail(
    `package.json version "${packageVersion}" is not semver (expected e.g. "1.1.3-zeta").`,
  );
}

const baseVersion = suffixMatch[1];
const codename = suffixMatch[2] ? suffixMatch[2].toUpperCase() : "ZETA";
const fullVersion = packageVersion;

console.log(
  `[sheytan-release] source of truth: package.json = ${fullVersion} ` +
    `(base ${baseVersion}, codename ${codename})`,
);

// ---------------------------------------------------------------------------
// 2. Verify (and, in sync mode, repair) every derived release surface.
// ---------------------------------------------------------------------------

let driftFound = 0;
let repaired = 0;

for (const target of targets) {
  const path = join(repoRoot, target.file);

  let content;
  try {
    content = readFileSync(path, "utf8");
  } catch (error) {
    fail(`unable to read ${target.label}: ${error.message}`);
  }

  const pattern = target.multiline
    ? new RegExp(target.pattern.source, "m")
    : target.pattern;

  const match = content.match(pattern);

  if (!match) {
    fail(
      `${target.label}: release metadata marker not found ` +
        `(expected something matching ${target.describe("<version>")}).`,
    );
  }

  const current = match[2];
  const wanted = target.expected(baseVersion, fullVersion);

  if (current === wanted) {
    console.log(
      `[sheytan-release] ${target.label}: OK (${target.describe(current)})`,
    );
    continue;
  }

  driftFound += 1;

  if (CHECK_ONLY) {
    console.error(
      `[sheytan-release] DRIFT in ${target.label}: expected ` +
        `${target.describe(wanted)}, found ${target.describe(current)}`,
    );
    continue;
  }

  // Sync mode: splice the new value into the matched span, leaving every
  // other byte (indentation, alignment, line endings) untouched.
  const start = match.index + match[1].length;
  const end = start + match[2].length;
  const updated = content.slice(0, start) + wanted + content.slice(end);

  writeFileSync(path, updated, "utf8");
  repaired += 1;

  console.log(
    `[sheytan-release] ${target.label}: repaired ` +
      `${target.describe(current)} -> ${target.describe(wanted)}`,
  );
}

// ---------------------------------------------------------------------------
// 3. Report.
// ---------------------------------------------------------------------------

if (CHECK_ONLY) {
  if (driftFound > 0) {
    console.error(
      `[sheytan-release] ${driftFound} release metadata drift(s) detected. ` +
        `Run "node scripts/release-version.mjs" (sync mode) to repair, ` +
        `then commit the result.`,
    );
    process.exit(1);
  }
  console.log("[sheytan-release] release metadata consistent (check mode).");
  process.exit(0);
}

if (repaired === 0) {
  console.log("[sheytan-release] release metadata consistent (sync mode).");
} else {
  console.log(
    `[sheytan-release] ${repaired} file(s) repaired from package.json ` +
      `version ${fullVersion}.`,
  );
}
