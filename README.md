# gograph
Project to recreate Lang Graph in Go lang

## Run the studio

Build the React Flow editor, then start the Go server:

```powershell
cd web
npm install
npm run build
cd ..
go run ./cmd/gograph
```

Open http://localhost:8080. The editor loads registered node types from `/registry/nodes`, serializes the canvas to the Phase 4 graph specification, and highlights each frontier from `/graph/stream`.
