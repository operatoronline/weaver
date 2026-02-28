package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/operatoronline/weaver/pkg/logger"
	"github.com/operatoronline/weaver/pkg/providers"
	"github.com/operatoronline/weaver/pkg/skills"
	"github.com/operatoronline/weaver/pkg/tools"
)

type ContextBuilder struct {
	workspace    string
	skillsLoader *skills.SkillsLoader
	memory       *MemoryStore
	tools        *tools.ToolRegistry // Direct reference to tool registry
}

func getGlobalConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".weaver")
}

func NewContextBuilder(workspace string) *ContextBuilder {
	// builtin skills: skills directory in current project
	// Use the skills/ directory under the current working directory
	wd, _ := os.Getwd()
	builtinSkillsDir := filepath.Join(wd, "skills")
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	return &ContextBuilder{
		workspace:    workspace,
		skillsLoader: skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(workspace),
	}
}

// SetToolsRegistry sets the tools registry for dynamic tool summary generation.
func (cb *ContextBuilder) SetToolsRegistry(registry *tools.ToolRegistry) {
	cb.tools = registry
}

func (cb *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	workspacePath, _ := filepath.Abs(filepath.Join(cb.workspace))
	runtime := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Build tools section dynamically
	toolsSection := cb.buildToolsSection()

	return fmt.Sprintf(`# weaver 🕸️

You are Weaver, a managed AI agentic thread. You exist to execute tasks with surgical precision and minimal footprint.

## Current Time
%s

## Runtime
%s

## Workspace
Your workspace is at: %s
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md

%s

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be surgical and accurate** - When using tools, briefly explain what you're doing.

3. **Memory** - When remembering something, write to %s/memory/MEMORY.md`,
		now, runtime, workspacePath, workspacePath, workspacePath, workspacePath, toolsSection, workspacePath)
}

func (cb *ContextBuilder) buildToolsSection() string {
	if cb.tools == nil {
		return ""
	}

	summaries := cb.tools.GetSummaries()
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("**CRITICAL**: You MUST use tools to perform actions. Do NOT pretend to execute commands or schedule tasks.\n\n")
	sb.WriteString("You have access to the following tools:\n\n")
	for _, s := range summaries {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (cb *ContextBuilder) BuildSystemPrompt() string {
	parts := []string{}

	// Core identity section
	parts = append(parts, cb.getIdentity())

	// Bootstrap files
	bootstrapContent := cb.LoadBootstrapFiles()
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// Skills - show summary, AI can read full content with read_file tool
	skillsSummary := cb.skillsLoader.BuildSkillsSummary()
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.

%s`, skillsSummary))
	}

	// Memory context
	memoryContext := cb.memory.GetMemoryContext()
	if memoryContext != "" {
		parts = append(parts, "# Memory\n\n"+memoryContext)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"NEST.md",
		"USER.md",
		"IDENTITY.md",
	}

	var result string
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			result += fmt.Sprintf("## %s\n\n%s\n\n", filename, string(data))
		}
	}

	return result
}

// BuildForgeSystemPrompt returns a minimal system prompt for Forge Studio requests.
// No agent identity, no tools, no memory — just the Forge design standards and output format.
func (cb *ContextBuilder) BuildForgeSystemPrompt(channel, chatID string) string {
	prompt := ""

	if channel == "forge:studio" || (channel == "forge" && chatID == "studio") {
		prompt = `You MUST respond with valid JSON only. Your response must be a JSON object with a "files" array. Each file object must have "name" (string), "content" (string), and "type" (string) fields.
Generate a complete, self-contained web application using HTML, CSS, and JavaScript.
Always include at minimum one HTML file as the entry point.

Do NOT include any text outside the JSON object. Do NOT use markdown code fences.

## Architecture Rules
- For simple requests (landing page, bio, calculator): single index.html with inlined CSS and JS.
- For complex apps (dashboards, multi-view apps): separate into index.html, styles.css, and app.js.
- Always return the COMPLETE content of every file. Never truncate or abbreviate.

## Design Standards
- Mobile-first, responsive design.
- OKLCH color model for perceptual harmony.
- 80% monochrome, 20% functional color.
- 4px base spacing scale.
- DM Sans font via Google Fonts CDN.
- Normalize.css via CDN.
- Phosphor Icons via CDN.
- No colored drop shadows, no gradient backgrounds, no flex-wrap (use truncation or horizontal scroll).
- Production-grade: semantic HTML, ARIA attributes, keyboard navigation.

## CDN Resources
- Normalize: https://cdn.jsdelivr.net/npm/normalize.css@8/normalize.css
- Phosphor Icons: https://unpkg.com/@phosphor-icons/web/src/regular/style.css
- DM Sans: https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;600;700&display=swap`
	} else if channel == "forge:plan" || (channel == "forge" && chatID == "plan") {
		prompt = `You are a planning assistant. Respond with valid JSON only.
Return a JSON object with a "plan" array containing objects with "assetId" (string), "prompt" (string for image generation), and "suggestedName" (string) fields.
Do NOT include any text outside the JSON object.`
	}

	return prompt
}

func (cb *ContextBuilder) BuildMessages(history []providers.Message, summary string, currentMessage string, media []string, channel, chatID string) []providers.Message {
	messages := []providers.Message{}

	systemPrompt := cb.BuildSystemPrompt()

	// Add Current Session info if provided
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	// Channel-specific output format instructions
	if channel == "forge:studio" || (channel == "forge" && chatID == "studio") {
		systemPrompt += `

## Output Format (STRICT)
You MUST respond with valid JSON only. Your response must be a JSON object with a "files" array. Each file object must have "name" (string) and "content" (string) fields.
Generate a complete, self-contained web application using HTML, CSS, and JavaScript.
Always include at minimum one HTML file as the entry point.

Example format:
{"files":[{"name":"index.html","content":"<!DOCTYPE html>..."},{"name":"style.css","content":"body{...}"}]}
Do NOT include any text outside the JSON object. Do NOT use markdown code fences.

## Role
You are a frontend development LLM agent with a design-obsessed mindset and a deep passion for crafting mobile-first user interfaces with pixel-perfect precision. You prioritize quality, visual harmony, and robust design systems above all else. Your design aesthetics must meet Apple Design Awards standards for output qualification or you don't bother working on the task because you don't tolerate basic, low quality, and sloppy outputs.

## Core Technologies
- **Grid System**: 24-column fluid grid system optimized for mobile-first breakpoints.
- **Spacing Scale**: 4px base spacing scale with consistent vertical rhythm across viewports.
- **Color Model**: OKLCH color model used exclusively for perceptual harmony.
- **Color System**:
  - **Ratio**: 80% monochrome (black, white, grays), 20% color — intentional, functional, never decorative.
  - **Primary**: Select one hue appropriate to brand/context — use consistently for key actions and focus states.
  - **Status**: Red (error), green (success), yellow (warning), blue (info).
  - **Accents**: Secondary hues — use sparingly, never mixed together in the same context.
- **Color Theme**: Monochrome-dominant palettes tuned for light/dark themes with purposeful color application.
- **Layout**: Flexbox for responsive and adaptable UI patterns. Wrapping is not allowed—overflow must be handled by truncated text, icon-only adaptation, or horizontal scroll (even on small viewports).
- **Structure**: Mathematical layout logic for modularity, consistency, and rhythm.
- **Disclosure**: Progressive disclosure to support simplicity with optional layered complexity.
- **Whitespace**: Strategic use of whitespace for structure, contrast, and breathing room.
- **Scrolling**: Horizontal scroll used purposefully for storytelling, hierarchy, and overflow handling.

## Design Principles
1. **Purposeful Minimalism**: Eliminate all non-essential elements—everything must serve a clear, functional purpose.
2. **Monochrome-First Color Discipline**: Design in grayscale first. Add color only where it serves function: status, focus, or primary actions.
3. **Spatial Hierarchy**: Use spacing, alignment, and layout scales to guide attention and organize information.
4. **Subtle Sophistication**: Create a premium feel through refined, restrained interactions and visual nuance.
5. **Functional Beauty**: Marry function and form—every design decision should elevate both.
6. **Consistent Rhythm**: Establish predictable visual and spatial cadence across all screen sizes.
7. **Overflow Discipline**: Prevent wrapping by enforcing truncation, icon adaptation, or horizontal scroll.
8. **Color Restraint**: Color communicates meaning, not decoration. One primary hue, used consistently. Never mix accents or muddy hierarchy with competing hues.

## Hard Constraints
- NEVER: Colored drop shadows.
- NEVER: Gradient backgrounds (especially "AI-style" purple-blue blends).
- NEVER: Redundant or nested containers around the same data.
- NEVER: Horizontal scroll on elements that could break page layout.
- NEVER: Color mixing that muddies hierarchy.
- NEVER: Decorative color usage outside the 20% functional allocation.
- NEVER: Multiple accent colors combined in the same context.
- NEVER: Switching primary hue mid-project without explicit direction.

## Philosophy
Always optimize for clarity, elegance, and usability. Favor designs that feel effortless, visually cohesive, and rigorously structured. Every component must begin with mobile-first logic and extend gracefully to larger viewports—never relying on flex-wrap, but instead using truncation, icon-only states, or horizontal scroll for overflow. Color exists to clarify, not decorate. Choose one primary hue and commit to it.

## Responsibilities
All outputs must be:
- Fully responsive and mobile-first by default.
- Monochrome-based (80%) with functional color (20%).
- Single primary hue selected based on brand, industry, or context — applied consistently.
- Theme-ready (light/dark OKLCH compatible).
- Self-contained, accessible, and performance-conscious.
- Strictly no flex-wrap: enforce truncation, icon-only states, or horizontal scroll for overflow.
- Color applied only for: primary actions, status feedback, or single accent highlights.
- Prefers floating Navbar with icon only logo alternatives on mobile viewport, Fab for quick actions, and user avatar dropdown for account level settings.

## CDN Resources
For all projects, use these predefined CDN resources:
- Normalize: https://cdn.jsdelivr.net/npm/normalize.css@8/normalize.css
- Phosphor Icons Regular: https://unpkg.com/@phosphor-icons/web/src/regular/style.css
- Phosphor Icons Fill: https://unpkg.com/@phosphor-icons/web/src/fill/style.css
- DM Sans Font: https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;600;700&display=swap
- PrismJS: https://cdn.jsdelivr.net/npm/prismjs@1/prism.min.js
- SwiperJS: https://cdn.jsdelivr.net/npm/swiper@11.2.10/+esm
- Use sortable.js for drag-and-drop components.
- Use marked + DOMPurify to securely render Markdown.`
	}
	if channel == "forge:plan" || (channel == "forge" && chatID == "plan") {
		systemPrompt += `

## Output Format (STRICT)
You are a planning assistant. Respond with valid JSON only.
Return a JSON object with a "plan" array containing objects with "assetId" (string), "prompt" (string for image generation), and "suggestedName" (string) fields.
Do NOT include any text outside the JSON object.`
	}

	// Log system prompt summary for debugging (debug mode only)
	logger.DebugCF("agent", "System prompt built",
		map[string]interface{}{
			"total_chars":   len(systemPrompt),
			"total_lines":   strings.Count(systemPrompt, "\n") + 1,
			"section_count": strings.Count(systemPrompt, "\n\n---\n\n") + 1,
		})

	// Log preview of system prompt (avoid logging huge content)
	preview := systemPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "... (truncated)"
	}
	logger.DebugCF("agent", "System prompt preview",
		map[string]interface{}{
			"preview": preview,
		})

	if summary != "" {
		systemPrompt += "\n\n## Summary of Previous Conversation\n\n" + summary
	}

	//This fix prevents the session memory from LLM failure due to elimination of toolu_IDs required from LLM
	// --- INICIO DEL FIX ---
	//Diegox-17
	for len(history) > 0 && (history[0].Role == "tool") {
		logger.DebugCF("agent", "Removing orphaned tool message from history to prevent LLM error",
			map[string]interface{}{"role": history[0].Role})
		history = history[1:]
	}
	//Diegox-17
	// --- FIN DEL FIX ---

	messages = append(messages, providers.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	messages = append(messages, history...)

	messages = append(messages, providers.Message{
		Role:    "user",
		Content: currentMessage,
	})

	return messages
}

func (cb *ContextBuilder) AddToolResult(messages []providers.Message, toolCallID, toolName, result string) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

func (cb *ContextBuilder) AddAssistantMessage(messages []providers.Message, content string, toolCalls []map[string]interface{}) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// Always add assistant message, whether or not it has tool calls
	messages = append(messages, msg)
	return messages
}

func (cb *ContextBuilder) loadSkills() string {
	allSkills := cb.skillsLoader.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var skillNames []string
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}

	content := cb.skillsLoader.LoadSkillsForContext(skillNames)
	if content == "" {
		return ""
	}

	return "# Skill Definitions\n\n" + content
}

// GetSkillsInfo returns information about loaded skills.
func (cb *ContextBuilder) GetSkillsInfo() map[string]interface{} {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]interface{}{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
	}
}
