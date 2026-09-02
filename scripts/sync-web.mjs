import { cp, mkdir, readdir } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(scriptDir, "..");

const sourceDir = resolve(projectRoot, "dist");
const stagingDir = resolve(projectRoot, "web", "static-react");

async function main() {
	await mkdir(sourceDir, { recursive: true });

	const sourceEntries = await readdir(sourceDir, {
		withFileTypes: true,
	});

	if (sourceEntries.length === 0) {
		throw new Error(
			"Vite produced an empty dist directory. Run the frontend build first.",
		);
	}

	await mkdir(stagingDir, {
		recursive: true,
	});

	await cp(sourceDir, stagingDir, {
		recursive: true,
		force: true,
	});

	console.log(`Frontend staged safely at: ${stagingDir}`);
	console.log(
		"web/static was left untouched; React integration remains non-destructive until verified.",
	);
}

await main();
