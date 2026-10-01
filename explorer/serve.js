import http from 'http';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import getPort from 'get-port';
import open from 'open';

// Convert __dirname equivalent in ES modules
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

async function startServer() {
    const publicDir = path.resolve(__dirname, 'public');
    const protoDir = path.resolve(__dirname, '../proto');
    const dataDir = process.argv[2];

    console.log("publicDir", publicDir)
    console.log("protoDir", protoDir)
    console.log("dataDir", dataDir)

    if (!dataDir) {
        console.error("Usage: npm start -- <path-to-data-dir>");
        process.exit(1);
    }

    const port = await getPort();

    const server = http.createServer((req, res) => {
        console.log("req.url", req.url)
        let filePath;
        // Route on the pathname so query strings (?errors=1) do not 404.
        const pathname = new URL(req.url, 'http://localhost').pathname;

        if (pathname.startsWith('/data/')) {
            filePath = path.join(dataDir, pathname.replace('/data', ''));
        } else if (pathname === '/graph.proto') {
            filePath = path.join(protoDir, 'graph.proto');
        } else if (pathname === '/' || pathname === '/explorer.html') {
            filePath = path.join(publicDir, 'explorer.html');
        } else {
            filePath = path.join(publicDir, pathname);
        }

        fs.readFile(filePath, (err, content) => {
            if (err) {
                res.writeHead(404, { 'Content-Type': 'text/plain' });
                res.end('File Not Found');
            } else {
                res.writeHead(200, { 'Content-Type': getContentType(filePath) });
                res.end(content);
            }
        });
    });

    // A FAILED run writes nodes_errors.pb / adjacency_lists_errors.pb instead of
    // nodes_000000_of_000000.pb; tell the page which set to load.
    const isErrorRun = fs.existsSync(path.join(dataDir, 'nodes_errors.pb'))
        && !fs.existsSync(path.join(dataDir, 'nodes_000000_of_000000.pb'));

    server.listen(port, () => {
        const url = `http://localhost:${port}/explorer.html${isErrorRun ? '?errors=1' : ''}`;
        console.log(`Serving:`);
        console.log(`  ${url}`);
        console.log(`  http://localhost:${port}/data/ -> ${dataDir}`);

        open(url);
    });
}

function getContentType(filePath) {
    const ext = path.extname(filePath).toLowerCase();
    const map = {
        '.html': 'text/html',
        '.js': 'application/javascript',
        '.css': 'text/css',
        '.json': 'application/json',
        '.png': 'image/png',
        '.jpg': 'image/jpeg',
        '.gif': 'image/gif',
        '.svg': 'image/svg+xml',
    };
    return map[ext] || 'application/octet-stream';
}

startServer();
