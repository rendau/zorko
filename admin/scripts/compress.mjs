// Writes .br and .gz copies next to the text files of the build. The backend
// serves them by Accept-Encoding (internal/app/spa.go): it compresses nothing
// itself, so files without a copy go out as is.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { extname, join } from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

const distDir = process.argv[2] ?? "dist";
const extensions = new Set([".html", ".js", ".css", ".svg", ".json", ".txt", ".map"]);

const encoders = [
  { ext: ".br", encode: (b) => brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }) },
  { ext: ".gz", encode: (b) => gzipSync(b, { level: 9 }) }
];

const files = readdirSync(distDir, { recursive: true, withFileTypes: true })
  .filter((entry) => entry.isFile() && extensions.has(extname(entry.name)))
  .map((entry) => join(entry.parentPath, entry.name));

for (const file of files) {
  const body = readFileSync(file);
  for (const { ext, encode } of encoders) {
    const encoded = encode(body);
    // A tiny file may grow when compressed; then the original is served.
    if (encoded.length < body.length) {
      writeFileSync(file + ext, encoded);
    }
  }
}

console.log(`compressed ${files.length} files in ${distDir}`);
