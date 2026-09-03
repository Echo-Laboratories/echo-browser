package page

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Echo-Laboratories/echo-browser/pkg/chrome"
)

func (p *Page) enable(ctx context.Context) error {
	p.mu.Lock()
	done := p.enabled
	p.mu.Unlock()
	if done {
		return nil
	}

	p.mu.Lock()
	ua, ver := p.chromeUA, p.chromeVer
	p.mu.Unlock()
	if ua != "" && ver != "" {
		if _, err := p.call.Call(ctx, "Emulation.setUserAgentOverride", chrome.UAOverride(ua, ver)); err != nil {
			return err
		}
	}

	if _, err := p.call.Call(ctx, "Page.enable", nil); err != nil {
		return err
	}
	if _, err := p.call.Call(ctx, "Page.setLifecycleEventsEnabled", map[string]any{"enabled": true}); err != nil {
		return err
	}

	p.call.On("Page.frameNavigated", p.onFrameNavigated)

	frameID, err := p.mainFrameID(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.frameID = frameID
	p.enabled = true
	p.mu.Unlock()
	return nil
}

func (p *Page) onFrameNavigated(params json.RawMessage) {
	var ev struct {
		Frame struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
		} `json:"frame"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	if ev.Frame.ParentID != "" {
		return
	}
	p.mu.Lock()
	p.frameID = ev.Frame.ID
	p.contextID = 0
	p.mu.Unlock()
}

func (p *Page) invalidateWorld() {
	p.mu.Lock()
	p.contextID = 0
	p.mu.Unlock()
}

func (p *Page) mainFrameID(ctx context.Context) (string, error) {
	raw, err := p.call.Call(ctx, "Page.getFrameTree", nil)
	if err != nil {
		return "", err
	}
	var res struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("page: frame tree: %w", err)
	}
	if res.FrameTree.Frame.ID == "" {
		return "", fmt.Errorf("page: empty frame id")
	}
	return res.FrameTree.Frame.ID, nil
}

func (p *Page) isolatedContext(ctx context.Context) (int, error) {
	if err := p.enable(ctx); err != nil {
		return 0, err
	}
	p.mu.Lock()
	if p.contextID != 0 {
		id := p.contextID
		p.mu.Unlock()
		return id, nil
	}
	frameID := p.frameID
	world := p.worldName
	p.mu.Unlock()

	if frameID == "" {
		var err error
		frameID, err = p.mainFrameID(ctx)
		if err != nil {
			return 0, err
		}
		p.mu.Lock()
		p.frameID = frameID
		p.mu.Unlock()
	}

	raw, err := p.call.Call(ctx, "Page.createIsolatedWorld", map[string]any{
		"frameId":             frameID,
		"worldName":           world,
		"grantUniveralAccess": true,
	})
	if err != nil {
		return 0, fmt.Errorf("page: isolated world: %w", err)
	}
	var res struct {
		ExecutionContextID int `json:"executionContextId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("page: isolated world decode: %w", err)
	}
	if res.ExecutionContextID == 0 {
		return 0, fmt.Errorf("page: isolated world returned contextId 0")
	}
	p.mu.Lock()
	p.contextID = res.ExecutionContextID
	p.mu.Unlock()
	return res.ExecutionContextID, nil
}
