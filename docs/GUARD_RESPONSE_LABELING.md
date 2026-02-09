# Guard Response Labeling

This document describes how guards label responses for DIFC (Decentralized Information Flow Control) enforcement in the MCP Gateway.

## DIFC Label Rules

DIFC uses two types of labels to control information flow:

### Secrecy Labels

Secrecy labels prevent unauthorized writes ("no write down"):

| Operation | Rule | Example |
|-----------|------|---------|
| **Read** | Agent must have ≥ resource secrecy tags | Resource `S_r={'secret'}` requires agent to have `S_a={'secret'}` |
| **Write** | Resource must have ≥ agent secrecy tags | Agent with `S_a={'secret'}` can only write to resources with `S_r={'secret'}` |

**Intuition**: Secrecy tags track what sensitive data an agent has seen. Reading secret data "taints" the agent, and tainted agents cannot leak data to less-secret destinations.

### Integrity Labels

Integrity labels prevent untrusted reads ("no read down"):

| Operation | Rule | Example |
|-----------|------|---------|
| **Read** | Resource must have ≥ agent integrity tags | Agent with `I_a={'verified'}` can only read from resources with `I_r={'verified'}` |
| **Write** | Agent must have ≥ resource integrity tags | Resource `I_r={'trusted'}` requires agent to have `I_a={'trusted'}` |

**Intuition**: Integrity tags track trustworthiness. Reading untrusted data "degrades" the agent's integrity, and degraded agents cannot write to high-integrity destinations.

### Flow Rules Summary

```
Read:  resource.secrecy  ⊆ agent.secrecy    (agent has clearance)
       resource.integrity ⊇ agent.integrity  (agent trusts resource)

Write: agent.secrecy    ⊆ resource.secrecy  (no information leak)
       agent.integrity  ⊇ resource.integrity (agent is trustworthy)
```

## DIFC Modes

The gateway supports three enforcement modes:

1. **Strict**: 

Agent labels are NEVER updated. 

For each tool call, the gateway first calls `LabelResource()` to get resource labels and operation type (i.e., read, write, read-write). 

If the operation is a read, the gateway makes the tool call and then calls `LabelResponse()` to get fine-grained labels for the response. The Reference Monitor then checks DIFC rules for each item and blocks the entire response if any item violates the rules.

If the operation is read-write or write, then the Reference Monitor checks DIFC rules based on resource labels before the tool call, and blocks if the rules are violated. For read-write and write operations, `LabelResponse()` is NOT called. 

2. **Filter**: 

Agent labels are NEVER updated. 

For each tool call, the gateway first calls `LabelResource()` to get resource labels and operation type (i.e., read, write, read-write). 

If the operation is a read, the gateway makes the tool call and then calls `LabelResponse()` to get fine-grained labels for the response. The Reference Monitor then checks DIFC rules for each item and removes any items that violate the rules from the response (instead of blocking the entire response). This allows agents to still get access to items they are authorized for, while filtering out unauthorized items.

If the operation is read-write or write, then the Reference Monitor checks DIFC rules based on resource labels before the tool call, and blocks if the rules are violated. If the rules are not violated, the tool call proceeds. For read-write operations, the Reference Monitor calls `LabelResponse()` to get fine-grained labels for the response. The Reference Monitor then checks DIFC rules for each item and removes any items that violate the rules from the response (instead of blocking the entire response). This allows agents to still get access to items they are authorized for, while filtering out unauthorized items. For write operations in filter mode, `LabelResponse()` is NOT called.

3. **Propagate**: 

Agent labels are may be updated based on the labels of data they access. However, tool calls will only ever add tags to the agent's secrecy labels and remove tags from the agent's integrity labels, to ensure that agents can only become more restricted over time.

For each tool call, the gateway first calls `LabelResource()` to get resource labels and operation type (i.e., read, write, read-write). 

If the operation is a read, the gateway makes the tool call and then calls `LabelResponse()` to get fine-grained labels for the response. For each item in the response, the Reference Monitor sets the agent's secrecy label to the union of the agent's current secrecy label and the item's secrecy label and sets the agent's integrity label to the intersection of the agent's current integrity label and the item's integrity label. 

If the operation is read-write or write, then the Reference Monitor checks DIFC rules based on resource labels before the tool call, and blocks if the rules are violated. If the rules are not violated, the tool call proceeds. For read-write operations, the Reference Monitor calls `LabelResponse()` to get fine-grained labels for the response. For each item in the response, the Reference Monitor sets the agent's secrecy label to the union of the agent's current secrecy label and the item's secrecy label and sets the agent's integrity label to the intersection of the agent's current integrity label and the item's integrity label.  For write operations in propagate mode, `LabelResponse()` is NOT called.

## Overview

Guards implement two labeling methods:

1. **`LabelResource()`** - Called BEFORE the backend request to determine:
   - Resource labels (secrecy/integrity requirements)
   - Operation type (read, write, read-write)

2. **`LabelResponse()`** - Called AFTER the backend request to provide:
   - Fine-grained per-item labels (for collections)
   - Or `nil` to use resource labels for entire response

## Supported Response Labeling Formats

The gateway supports multiple formats for `LabelResponse()` return values.

### 1. Nil Response

Return `nil` to use the resource labels from `LabelResource()` for the entire response.

**Use when**: The coarse-grained resource labels are sufficient (single resource or uniform collection).

### 2. Path-Based Labeling (Preferred for Collections)

Apply different labels to specific items in a collection. Return JSON with this structure:

```json
{
  "labeled_paths": [
    {
      "path": "/items/0",
      "labels": {
        "description": "Public repository",
        "secrecy": ["public"],
        "integrity": ["github_verified"]
      }
    },
    {
      "path": "/items/1", 
      "labels": {
        "description": "Private repository user/secret-project",
        "secrecy": ["repo_private", "private:user/secret-project"],
        "integrity": ["github_verified"]
      }
    }
  ],
  "default_labels": {
    "secrecy": ["public"],
    "integrity": ["untrusted"]
  },
  "items_path": "/items"
}
```

**Behavior**: Labels are associated with JSON Pointer paths (RFC 6901) rather than copying data.

**Use when**: Labeling collections where items have different sensitivity levels.

**Fields**:

| Field | Type | Description |
|-------|------|-------------|
| `labeled_paths` | array | Path → labels mappings |
| `labeled_paths[].path` | string | JSON Pointer (RFC 6901) to the item |
| `labeled_paths[].labels` | object | Labels for this path |
| `labeled_paths[].labels.description` | string | Human-readable description (optional) |
| `labeled_paths[].labels.secrecy` | string[] | Secrecy tags |
| `labeled_paths[].labels.integrity` | string[] | Integrity tags |
| `default_labels` | object | Labels for items not explicitly listed (optional) |
| `items_path` | string | JSON Pointer to the collection (e.g., `/items`, `""` for root array) |

### 3. SimpleLabeledData (Go Guards Only)

For native Go guards, return a `SimpleLabeledData` struct to override resource labels:

```go
return &difc.SimpleLabeledData{
    Data:   result,  // The response data
    Labels: &difc.LabeledResource{
        Description: "API response",
        Secrecy:     secrecyLabel,
        Integrity:   integrityLabel,
    },
}, nil
```

**Note**: This format is not available for WASM guards. Use `nil` with appropriate `LabelResource()` labels instead.

## Format Detection (WASM Guards)

For WASM guards, the gateway auto-detects the format based on `LabelResponse()` output:

1. If response contains `labeled_paths` key → Parse as **PathLabeledData**
2. If response contains `items` array → Parse as **CollectionLabeledData** (legacy)
3. Empty or other response → Treat as `nil` (use resource labels)

**Note**: SimpleLabeledData format detection is not currently implemented for WASM guards. Use `nil` response with appropriate `LabelResource()` labels, or use path-based labeling.

## JSON Pointer Syntax (RFC 6901)

Path-based labeling uses JSON Pointer syntax:

| Pointer | Targets |
|---------|---------|
| `""` or `/` | Root document |
| `/items` | The `items` property |
| `/items/0` | First element of `items` array |
| `/items/5` | Sixth element of `items` array |
| `/data/users/0` | First user in nested structure |

**Escaping**:
- `~0` represents `~`
- `~1` represents `/`

## Example: GitHub Repository Search

For a `search_repositories` response:

```json
{
  "items": [
    {"full_name": "user/public-repo", "private": false},
    {"full_name": "user/private-repo", "private": true}
  ]
}
```

Guard returns:

```json
{
  "labeled_paths": [
    {
      "path": "/items/0",
      "labels": {
        "description": "user/public-repo",
        "secrecy": ["public"],
        "integrity": ["github_verified"]
      }
    },
    {
      "path": "/items/1",
      "labels": {
        "description": "user/private-repo",
        "secrecy": ["repo_private", "private:user/private-repo"],
        "integrity": ["github_verified"]
      }
    }
  ],
  "items_path": "/items"
}
```

## Filtering Behavior

After `LabelResponse()`, the Reference Monitor applies fine-grained filtering based on the enforcement mode:

1. **Strict mode**: Read requests are blocked at the coarse-grained check (Phase 2) if agent labels don't satisfy resource labels. `LabelResponse()` is not called for blocked requests.

2. **Filter mode**: Coarse-grained check is skipped for reads. After backend call, `LabelResponse()` provides per-item labels, and inaccessible items are filtered out. Agent labels are NOT updated.

3. **Propagate mode**: Same as filter mode, but agent labels are updated to include the labels of data they accessed. This enables information flow tracking.

## Performance Considerations

| Format | Data Copying | Memory | Best For |
|--------|-------------|--------|----------|
| `nil` | None | Minimal | Uniform labels |
| `SimpleLabeledData` | None | Low | Single items or uniform collections |
| `PathLabeledData` | None | Low | **Collections with mixed labels** |

**Recommendation**: Use path-based labeling for collections where items have different sensitivity levels.
