package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/operatoronline/weaver/pkg/logger"
)

type ImageTool struct {
	workspace string
	apiKey    string
}

func NewImageTool(workspace, apiKey string) *ImageTool {
	return &ImageTool{
		workspace: workspace,
		apiKey:    apiKey,
	}
}

func (t *ImageTool) Name() string {
	return "generate_image"
}

func (t *ImageTool) Description() string {
	return "Generate or edit images using Gemini 3 Pro Image (Nano Banana Pro)."
}

func (t *ImageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Image description or edit instructions",
			},
			"resolution": map[string]interface{}{
				"type":        "string",
				"description": "Output resolution (1K, 2K, 4K)",
				"enum":        []string{"1K", "2K", "4K"},
				"default":     "1K",
			},
			"aspect_ratio": map[string]interface{}{
				"type":        "string",
				"description": "Aspect ratio (1:1, 3:4, 4:3, 9:16, 16:9)",
				"enum":        []string{"1:1", "3:4", "4:3", "9:16", "16:9"},
				"default":     "1:1",
			},
		},
		"required": []string{"prompt"},
	}
}

const ENHANCED_IMAGE_PROMPT_WRAPPER = `Subject: [USER_PROMPT].
Camera Gear: Leica M11, Hasselblad X2D, 85mm Prime f/1.2.
Lighting: Rembrandt lighting, volumetric god rays, high-key studio, cinematic rim light.
Aesthetic: Photorealistic, 8k resolution, highly detailed skin texture, soft bokeh background, subsurface scattering, Octane Render, Ray-traced global illumination.
Composition: Minimalist, strong central subject, purposeful negative space, high-end editorial and cinematic standards. 
Style: Avoid generic "AI-style" neon/purple tropes. No purple-blue gradients. No colored shadows. 

User Prompt: `

func (t *ImageTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	prompt, _ := args["prompt"].(string)
	resolution, _ := args["resolution"].(string)
	if resolution == "" {
		resolution = "1K"
	}

	// Enhance the prompt with technical art direction
	enhancedPrompt := fmt.Sprintf("%s\"%s\"", ENHANCED_IMAGE_PROMPT_WRAPPER, prompt)

	// Create output filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("gen-%s.png", timestamp)
	outputPath := filepath.Join(t.workspace, "generated", filename)
	os.MkdirAll(filepath.Join(t.workspace, "generated"), 0755)

	// Use absolute path to the generation script
	scriptPath := "/usr/lib/node_modules/openclaw/skills/nano-banana-pro/scripts/generate_image.py"

	cmdArgs := []string{
		"run", scriptPath,
		"--prompt", enhancedPrompt,
		"--filename", outputPath,
		"--resolution", resolution,
		"--api-key", t.apiKey,
	}

	logger.InfoCF("tool", "Generating image", map[string]interface{}{
		"prompt":     prompt,
		"resolution": resolution,
		"output":     outputPath,
	})

	cmd := exec.CommandContext(ctx, "uv", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResult(fmt.Sprintf("Image generation failed: %v\nOutput: %s", err, string(output)))
	}

	// Read the generated image and convert to data URI
	imgData, err := os.ReadFile(outputPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read generated image: %v", err))
	}

	dataURIBase64 := base64.StdEncoding.EncodeToString(imgData)
	dataURI := fmt.Sprintf("data:image/png;base64,%s", dataURIBase64)

	// Clean up the file after reading
	os.Remove(outputPath)

	res := NewToolResult(fmt.Sprintf("IMAGE_GENERATED: %s", dataURI))
	res.ForUser = fmt.Sprintf("I've generated the image based on your prompt: %q", prompt)
	return res
}
