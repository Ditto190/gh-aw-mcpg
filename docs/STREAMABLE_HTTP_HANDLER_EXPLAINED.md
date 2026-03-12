# StreamableHTTPHandler: How It Works

> **⚠️ HISTORICAL DOCUMENT NOTE (March 2026):**
>
> This document describes architectural limitations that **have been solved** in the current implementation. The gateway now uses a `filteredServerCache` (see `internal/server/routed.go:33-74`) that preserves SDK protocol state across HTTP requests by caching server instances per (backend, session) pair.
>
> **Current Implementation Status:**
> - ✅ Protocol state IS preserved across HTTP requests
> - ✅ SDK Server instances are cached and reused per session
> - ✅ Session-scoped SDK Server reuse supports both stateless and stateful backend interaction patterns
>
> This document is maintained for historical context to show the problem that was solved. For current architecture, see the implementation in `internal/server/routed.go` and `internal/server/transport.go`.

---

This document explains where the MCP SDK's `StreamableHTTPHandler` lives, what it does, and how it communicates with backend MCP servers.

## TL;DR

- **StreamableHTTPHandler**: Frontend only (gateway side), translates HTTP ↔ JSON-RPC
- **Backend**: Just a process receiving JSON-RPC via stdio, no HTTP awareness
- **What backend receives**: Plain JSON-RPC messages like `{"jsonrpc":"2.0","method":"tools/call",...}`
- **Protocol state**: Tracked separately on frontend (SDK Server) and backend (server code)
- **The issue**: New SDK Server instance per HTTP request = fresh protocol state, even though backend connection is reused

## Where Does StreamableHTTPHandler Live?

**Answer: Frontend only (gateway side)**

```
┌─────────────────────────────────────────┐
│         Gateway Process                  │
│  ┌────────────────────────────────────┐ │
│  │  StreamableHTTPHandler (Frontend)  │ │
│  │  - Receives HTTP POST requests     │ │
│  │  - Translates to JSON-RPC          │ │
│  │  - Creates SDK Server instance     │ │
│  │  - Tracks protocol state           │ │
│  └────────────────────────────────────┘ │
│              ↓ stdio pipes               │
└──────────────┼──────────────────────────┘
               │ JSON-RPC messages
               ↓
┌──────────────────────────────────────────┐
│    Backend Process (e.g., Serena)        │
│  - Receives JSON-RPC via stdin           │
│  - Sends JSON-RPC via stdout             │
│  - NO awareness of HTTP                  │
│  - NO awareness of StreamableHTTPHandler │
│  - Tracks its own state machine          │
└──────────────────────────────────────────┘
```

**Key Points:**
- Backend is just a process that speaks JSON-RPC over stdio
- Backend never sees HTTP requests, headers, or StreamableHTTPHandler
- Backend has no knowledge it's behind a gateway

## What Does StreamableHTTPHandler Do?

### Primary Function: HTTP ↔ JSON-RPC Translation

```
HTTP Request (from agent)
  POST /mcp/serena
  Body: {"jsonrpc":"2.0","method":"tools/call","params":{...}}
              ↓
   StreamableHTTPHandler
   - Creates SDK Server instance
   - Parses JSON-RPC from HTTP body
   - Routes to SDK Server methods
              ↓
   SDK Server instance
   - Validates protocol state (uninitialized → ready)
   - Formats as JSON-RPC message
              ↓
   Stdio pipes to backend
   - Writes: {"jsonrpc":"2.0","method":"tools/call",...}
              ↓
   Backend Process (Serena)
   - Reads JSON-RPC from stdin
   - Processes request
   - Validates its own state
   - Writes response to stdout
              ↓
   SDK Server instance
   - Reads JSON-RPC response from stdio
              ↓
   StreamableHTTPHandler
   - Translates to HTTP response
              ↓
HTTP Response (to agent)
  Body: {"jsonrpc":"2.0","result":{...}}
```

## What Gets Passed to the Backend?

**Answer: Only JSON-RPC messages, nothing about HTTP or protocol state**

### Example Flow:

**HTTP Request 1 (initialize):**
```
Agent sends HTTP:
  POST /mcp/serena
  Authorization: session-123
  Body: {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {"protocolVersion": "2024-11-05", ...}
  }

Gateway StreamableHTTPHandler:
  - Extracts session ID from Authorization header
  - Creates NEW SDK Server instance for this request
  - SDK Server state: uninitialized
  - Sees "initialize" method → Valid for uninitialized state
  - Transitions state: uninitialized → ready

Backend (Serena) receives via stdio:
  {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {"protocolVersion": "2024-11-05", ...}
  }

Backend does NOT receive:
  ❌ HTTP headers (Authorization, Content-Type, etc.)
  ❌ Session ID
  ❌ Frontend SDK protocol state
  ❌ Any indication this came via HTTP
```

**HTTP Request 2 (tools/call) - SAME session:**
```
Agent sends HTTP:
  POST /mcp/serena
  Authorization: session-123
  Body: {
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {"name": "search_code", ...}
  }

Gateway StreamableHTTPHandler:
  - Extracts session ID: session-123 (same as before)
  - Backend connection: ✅ REUSED (session pool works)
  - Creates NEW SDK Server instance for this request ❌
  - SDK Server state: uninitialized ❌
  - Sees "tools/call" method → Invalid for uninitialized state ❌
  - ERROR: "method 'tools/call' is invalid during session initialization"

Backend (Serena) NEVER receives this request
  ❌ Request blocked by frontend SDK protocol validation
```

## Protocol State: Frontend vs Backend

This is the critical distinction:

### Frontend Protocol State (SDK Server)
```
Location: Gateway process, SDK Server instance
Tracks: MCP protocol state machine
States: uninitialized → initializing → ready
Problem: NEW instance per HTTP request = always uninitialized
```

### Backend Protocol State (Server Implementation)
```
Location: Backend process (Serena/GitHub)
Tracks: Backend's own state machine
GitHub: NO state validation (stateless)
Serena: ENFORCES state validation (stateful)
```

### The Disconnect:

```
┌─────────────────────────────────────────────────────────────┐
│                      Gateway (Frontend)                      │
├─────────────────────────────────────────────────────────────┤
│  Request 1:                                                  │
│    SDK Server instance #1 (state: uninitialized)            │
│    Sees: initialize → Valid → State: ready ✅               │
│    Sends to backend: {"method":"initialize",...}            │
│                                                              │
│  Request 2 (same session):                                  │
│    SDK Server instance #2 (state: uninitialized) ❌         │
│    Sees: tools/call → Invalid → ERROR ❌                    │
│    Backend never receives this request                      │
└─────────────────────────────────────────────────────────────┘
                              ↓ stdio (persistent)
┌─────────────────────────────────────────────────────────────┐
│              Backend Process (Same process, reused ✅)       │
├─────────────────────────────────────────────────────────────┤
│  Received Request 1:                                         │
│    {"method":"initialize",...}                               │
│    Backend state: uninitialized → ready ✅                  │
│                                                              │
│  Request 2 would have been fine:                            │
│    Backend state: still ready ✅                            │
│    Would process {"method":"tools/call",...} successfully   │
│    But frontend SDK blocked it before backend saw it ❌     │
└─────────────────────────────────────────────────────────────┘
```

## Why GitHub Works But Serena Doesn't

### GitHub MCP Server (Stateless)

**Backend code doesn't validate protocol state:**
```typescript
// GitHub MCP Server
server.setRequestHandler(ListToolsRequestSchema, async () => {
  // NO state check - just process the request
  // Works regardless of whether initialize was called
  return { tools: [...] };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  // NO state check - just execute the tool
  return await executeTool(request.params.name, request.params.arguments);
});
```

**Result:**
```
Frontend SDK Server: uninitialized (wrong) ❌
Backend doesn't care: processes request anyway ✅
Works through gateway: ✅
```

### Serena MCP Server (Stateful)

**Backend code validates protocol state:**
```python
# Serena MCP Server
class SerenaServer:
    def __init__(self):
        self.state = "uninitialized"
    
    async def handle_initialize(self, params):
        self.state = "ready"
        return {"protocolVersion": "2024-11-05"}
    
    async def list_tools(self):
        if self.state != "ready":  # State validation
            raise Error("method 'tools/list' is invalid during session initialization")
        return {"tools": [...]}
    
    async def call_tool(self, name, arguments):
        if self.state != "ready":  # State validation
            raise Error("method 'tools/call' is invalid during session initialization")
        return await self._execute_tool(name, arguments)
```

**Result:**
```
Frontend SDK Server: uninitialized (wrong) ❌
Backend validates state: rejects request ❌
Fails through gateway: ❌
```

## Backend Connection vs Protocol State

This is crucial to understand:

### Backend Connection (Works Correctly ✅)
```
- Managed by SessionConnectionPool
- One persistent stdio connection per (backend, session)
- Same Docker container process
- Same stdin/stdout/stderr pipes
- Connection IS reused across HTTP requests ✅
```

### Protocol State (Doesn't Persist ❌)
```
- Managed by SDK Server instances
- New instance created per HTTP request
- Each instance starts in "uninitialized" state
- Protocol state NOT preserved across HTTP requests ❌
```

### Visual:
```
HTTP Request 1 (Authorization: session-123)
  → NEW SDK Server (state: uninitialized)
  → REUSED backend connection ✅
  → Same backend process ✅
  → {"method":"initialize"} sent

HTTP Request 2 (Authorization: session-123)
  → NEW SDK Server (state: uninitialized) ❌
  → REUSED backend connection ✅
  → Same backend process ✅
  → {"method":"tools/call"} blocked by SDK ❌
```

## The Architecture Issue

The SDK's `StreamableHTTPHandler` was designed for **stateless HTTP scenarios** where:
- Each HTTP request is completely independent
- No session state needs to persist
- Backend doesn't validate protocol state

It doesn't support **stateful backends** where:
- Protocol handshake must complete on the same session
- Backend validates that initialize was called before other methods
- Session state must persist across multiple HTTP requests

## Summary

**Where StreamableHTTPHandler lives:**
- Frontend only (gateway process)

**What it does:**
- Translates HTTP requests to JSON-RPC messages
- Creates SDK Server instances to handle protocol
- Sends JSON-RPC to backend via stdio

**What backend receives:**
- Plain JSON-RPC messages via stdin
- No HTTP, no headers, no session context
- No frontend protocol state information

**The problem:**
- ✅ Backend stdio connection properly reused
- ✅ Backend process state maintained correctly
- ❌ Frontend SDK Server instance recreated per request
- ❌ Frontend protocol state reset to uninitialized
- ✅ Stateless backends (GitHub) work because they don't care
- ❌ Stateful backends (Serena) fail because they validate state

**The limitation:**
- This is an SDK architectural pattern
- StreamableHTTPHandler doesn't support session persistence
- Backend connection pooling works, but SDK protocol state doesn't persist
- Would require SDK changes or bypassing StreamableHTTPHandler entirely
