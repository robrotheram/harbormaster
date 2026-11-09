package api

import (
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

type desktopRuntime interface {
	Desktop(http.ResponseWriter, *http.Request, string, string)
}
type consoleRuntime interface {
	Console(http.ResponseWriter, *http.Request, string, string)
}
type runtimeCapabilities interface {
	Capabilities(context.Context) (protocol.RuntimeCapability, error)
}
type runtimeAgentKinds interface {
	AgentKinds(context.Context) ([]string, error)
}
type matrixCredentialStatus interface {
	MatrixCredentialPresent(context.Context, string) bool
}
type runtimeMatrixCredentialWriter interface {
	MatrixCredential(context.Context, string, shared.HermesMatrix, string) error
}
type matrixActionRuntime interface {
	MatrixAction(context.Context, string, string, *shared.HermesMatrix) error
}
