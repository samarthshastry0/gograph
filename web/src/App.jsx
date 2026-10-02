import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  addEdge,
  useEdgesState,
  useNodesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

const API = import.meta.env.VITE_API_URL || 'http://localhost:8080'
const START = '__start__'
const END = '__end__'

const initialNodes = [
  { id: 'dispatch', type: 'graphNode', position: { x: 240, y: 70 }, data: { label: 'dispatch', nodeType: 'dispatch', config: {} } },
  { id: 'Race Car1', type: 'graphNode', position: { x: 40, y: 260 }, data: { label: 'Race Car1', nodeType: 'race_car', config: { name: 'Race Car1' } } },
  { id: 'Race Car2', type: 'graphNode', position: { x: 240, y: 260 }, data: { label: 'Race Car2', nodeType: 'race_car', config: { name: 'Race Car2' } } },
  { id: 'Race Car3', type: 'graphNode', position: { x: 440, y: 260 }, data: { label: 'Race Car3', nodeType: 'race_car', config: { name: 'Race Car3' } } },
  { id: 'join', type: 'graphNode', position: { x: 240, y: 470 }, data: { label: 'join', nodeType: 'join', config: {} } },
]

const initialEdges = [
  { id: 'start-dispatch', source: START, target: 'dispatch', markerEnd: { type: MarkerType.ArrowClosed } },
  ...['Race Car1', 'Race Car2', 'Race Car3'].map((id) => ({ id: `dispatch-${id}`, source: 'dispatch', target: id, markerEnd: { type: MarkerType.ArrowClosed } })),
  ...['Race Car1', 'Race Car2', 'Race Car3'].map((id) => ({ id: `${id}-join`, source: id, target: 'join', markerEnd: { type: MarkerType.ArrowClosed } })),
  { id: 'join-end', source: 'join', target: END, markerEnd: { type: MarkerType.ArrowClosed } },
]

function GraphNode({ data }) {
  const active = data.active
  return <div className={`graph-node ${active ? 'active' : ''}`}>
    <Handle type="target" position={Position.Top} />
    <span className="node-kind">{data.nodeType}</span>
    <strong>{data.label}</strong>
    <Handle type="source" position={Position.Bottom} />
  </div>
}

const nodeTypes = { graphNode: GraphNode }

function toSpec(nodes, edges) {
  return {
    entry: START,
    nodes: nodes.map(({ id, data }) => ({ id, type: data.nodeType, config: data.config || {} })),
    edges: edges.map(({ source, target }) => ({ from: source, to: target })),
    channels: [
      { key: 'messages', reducer: 'append' },
      { key: 'done', reducer: 'add' },
    ],
  }
}

async function readStream(response, onStep) {
  if (!response.body) throw new Error('The server did not return a stream')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
    const blocks = buffer.split('\n\n')
    buffer = blocks.pop() || ''
    blocks.forEach((block) => {
      const line = block.split('\n').find((item) => item.startsWith('data: '))
      if (line) onStep(JSON.parse(line.slice(6)))
    })
    if (done) break
  }
}

export default function App() {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const [palette, setPalette] = useState([])
  const [events, setEvents] = useState([])
  const [state, setState] = useState({})
  const [status, setStatus] = useState('Ready')
  const [error, setError] = useState('')

  useEffect(() => {
    fetch(`${API}/registry/nodes`)
      .then((response) => response.json())
      .then((payload) => setPalette(payload.types || []))
      .catch(() => setPalette(['dispatch', 'race_car', 'join']))
  }, [])

  const onConnect = useCallback((connection) => {
    setEdges((current) => addEdge({ ...connection, markerEnd: { type: MarkerType.ArrowClosed } }, current))
  }, [setEdges])

  const onDrop = useCallback((event) => {
    event.preventDefault()
    const nodeType = event.dataTransfer.getData('application/gograph-node')
    if (!nodeType) return
    const id = `${nodeType}-${nodes.length + 1}`
    const bounds = event.currentTarget.getBoundingClientRect()
    const position = { x: event.clientX - bounds.left - 70, y: event.clientY - bounds.top - 30 }
    setNodes((current) => [...current, { id, type: 'graphNode', position, data: { label: id, nodeType, config: nodeType === 'race_car' ? { name: id } : {} } }])
  }, [nodes.length, setNodes])

  const spec = useMemo(() => toSpec(nodes, edges), [nodes, edges])

  const run = async () => {
    setStatus('Running')
    setError('')
    setEvents([])
    setState({})
    try {
      const response = await fetch(`${API}/graph/stream`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ graph: spec, initial: { data: {} } }) })
      if (!response.ok) throw new Error((await response.json()).error || 'Graph request failed')
      await readStream(response, (event) => {
        setEvents((current) => [...current, event])
        setState(event.state?.data || {})
        setNodes((current) => current.map((node) => ({ ...node, data: { ...node.data, active: event.active.includes(node.id) } })))
      })
      setStatus('Complete')
    } catch (runError) {
      setStatus('Failed')
      setError(runError.message)
    }
  }

  return <main className="app-shell">
    <header className="topbar"><div><span className="eyebrow">PHASE 06 / GRAPH RUNTIME</span><h1>GoGraph <em>Studio</em></h1></div><button className="run-button" onClick={run}>Run graph <span>→</span></button></header>
    <section className="workspace">
      <aside className="sidebar"><div className="section-label">Node palette</div><p>Drag a registered behavior onto the canvas.</p>{palette.map((type) => <div className="palette-item" draggable key={type} onDragStart={(event) => event.dataTransfer.setData('application/gograph-node', type)}><span className="palette-dot" />{type}<span className="drag-mark">+</span></div>)}<div className="section-label inspector-label">Execution</div><div className="status"><span className={`status-dot ${status === 'Running' ? 'pulse' : ''}`} />{status}<span className="step-count">{events.length} steps</span></div>{error && <div className="error">{error}</div>}</aside>
      <div className="canvas-wrap" onDragOver={(event) => event.preventDefault()} onDrop={onDrop}><ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} fitView><Background color="#d7d0c2" gap={24} /><Controls /><MiniMap /></ReactFlow></div>
      <aside className="output"><div className="section-label">Live state</div><div className="state-list">{Object.entries(state).map(([key, value]) => <div className="state-row" key={key}><span>{key}</span><strong>{Array.isArray(value) ? value.length : String(value)}</strong></div>)}{!Object.keys(state).length && <div className="empty">Run the graph to inspect state.</div>}</div><div className="section-label trace-label">Trace</div><div className="trace">{events.map((event) => <div className="trace-row" key={event.step}><span>0{event.step + 1}</span><div>{event.active.map((name) => <b key={name}>{name}</b>)}</div></div>)}</div><details><summary>Graph JSON</summary><pre>{JSON.stringify(spec, null, 2)}</pre></details></aside>
    </section>
  </main>
}
