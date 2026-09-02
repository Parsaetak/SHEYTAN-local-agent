import { cp, mkdir, readdir, rm } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(scriptDir, "..");

const sourceDir = resolve(projectRoot, "dist");
const targetDir = resolve(projectRoot, "web", "static");

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

	await rm(targetDir, {
		recursive: true,
		force: true,
	});

	await mkdir(targetDir, {
		recursive: true,
	});

	await cp(sourceDir, targetDir, {
		recursive: true,
	});

	console.log(`Frontend synchronized: ${targetDir}`);
}

await main();
