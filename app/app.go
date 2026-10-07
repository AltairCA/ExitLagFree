package main

import (
	"context"

	"github.com/AltairCA/ExitLagFree/internal/client/core"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/profiles"
)

// App is bound to the frontend; every exported method is callable from JS
// as window.go.main.App.<Method>.
type App struct {
	ctx  context.Context
	core *core.Core
}

func NewApp() (*App, error) {
	c, err := core.New()
	if err != nil {
		return nil, err
	}
	return &App{core: c}, nil
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) GetState() core.State { return a.core.State(a.ctx) }

func (a *App) AddNode(link string) (core.Node, error) { return a.core.AddNode(a.ctx, link) }

func (a *App) RemoveNode(id string) error { return a.core.RemoveNode(a.ctx, id) }

func (a *App) RenameNode(id, name string) error { return a.core.RenameNode(id, name) }

func (a *App) PingNodes() []core.NodePing { return a.core.PingNodes(a.ctx) }

func (a *App) SetSelection(sel profiles.Selection) error { return a.core.SetSelection(sel) }

func (a *App) Connect(nodeID string) (proto.HelperStatus, error) {
	return a.core.Connect(a.ctx, nodeID)
}

func (a *App) Disconnect() (proto.HelperStatus, error) { return a.core.Disconnect(a.ctx) }

func (a *App) Compare(nodeID, target string) (core.Comparison, error) {
	return a.core.Compare(a.ctx, nodeID, target)
}

func (a *App) InstallHelper() error { return a.core.InstallHelper(a.ctx) }

func (a *App) UninstallHelper() error { return a.core.UninstallHelper(a.ctx) }
