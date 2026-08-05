// Package policyapi provides public interfaces for custom security policy engines.
package policyapi

import (
	"context"
	"net/http"
)

// EvaluationPhase represents the lifecycle phase of request processing.
type EvaluationPhase string

const (
	PhaseRequestHeaders  EvaluationPhase = "RequestHeaders"
	PhaseRequestBody     EvaluationPhase = "RequestBody"
	PhaseResponseHeaders EvaluationPhase = "ResponseHeaders"
)

// DecisionAction defines the action resulting from policy evaluation.
type DecisionAction string

const (
	ActionAllow DecisionAction = "Allow"
	ActionDeny  DecisionAction = "Deny"
	ActionBlock DecisionAction = "Block"
	ActionAudit DecisionAction = "Audit"
)

// Decision represents the result of evaluating a policy.
type Decision struct {
	Action      DecisionAction
	StatusCode  int
	Reason      string
	RuleID      string
	Description string
}

// PolicyContext provides request details during evaluation.
type PolicyContext struct {
	Request   *http.Request
	ClientIP  string
	RouteID   string
	Script    string
	Attributes map[string]interface{}
}

// Engine is the public interface for security policy evaluators.
type Engine interface {
	Evaluate(ctx context.Context, phase EvaluationPhase, pctx *PolicyContext) Decision
}
