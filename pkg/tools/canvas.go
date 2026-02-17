package tools

import (
	"context"
	"fmt"
	"sync"
)

// UICommand represents a command to be executed by the frontend
type UICommand struct {
	Command string                 `json:"command"`
	Args    map[string]interface{} `json:"args"`
}

// CanvasTool implements UI control for the Nest interface
type CanvasTool struct {
	mu       sync.Mutex
	commands []UICommand
}

func NewCanvasTool() *CanvasTool {
	return &CanvasTool{
		commands: make([]UICommand, 0),
	}
}

func (t *CanvasTool) Name() string {
	return "canvas"
}

func (t *CanvasTool) Description() string {
	return "Control the Nest UI interface (create nodes, organize canvas, etc.)"
}

func (t *CanvasTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"create_node", "update_node", "delete_node", "clear"},
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Type of node (text, code, image, video)",
				"enum":        []string{"text", "code", "image", "video"},
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Title of the node",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content of the node (Markdown, code, or URL)",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the node (for update/delete)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *CanvasTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	action, _ := args["action"].(string)
	
	cmd := UICommand{
		Command: action,
		Args:    args,
	}
	t.commands = append(t.commands, cmd)

	return NewToolResult(fmt.Sprintf("UI action %q queued for execution", action))
}

func (t *CanvasTool) FlushCommands() []UICommand {
	t.mu.Lock()
	defer t.mu.Unlock()
	cmds := t.commands
	t.commands = make([]UICommand, 0)
	return cmds
}
