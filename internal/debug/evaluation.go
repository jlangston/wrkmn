package debug

import (
	"encoding/json"
	"fmt"
)

// Evaluator handles expression evaluation (REPL)
type Evaluator struct {
	session *Session
}

// NewEvaluator creates a new evaluator
func NewEvaluator(session *Session) *Evaluator {
	return &Evaluator{session: session}
}

// Evaluate evaluates an expression in the current context
func (e *Evaluator) Evaluate(expression string) (*EvaluationResult, error) {
	// If paused, evaluate on the current call frame
	if e.session.state.GetState() == StatePaused {
		return e.EvaluateOnCallFrame(expression, "")
	}

	return e.EvaluateGlobal(expression)
}

// EvaluateGlobal evaluates an expression in the global context
func (e *Evaluator) EvaluateGlobal(expression string) (*EvaluationResult, error) {
	params := map[string]any{
		"expression":            expression,
		"generatePreview":       true,
		"includeCommandLineAPI": true,
		"returnByValue":         false,
	}

	result, err := e.session.Call("Runtime.evaluate", params)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	return parseEvaluationResult(result)
}

// EvaluateOnCallFrame evaluates an expression on a specific call frame
func (e *Evaluator) EvaluateOnCallFrame(expression string, callFrameID string) (*EvaluationResult, error) {
	// Use the topmost frame if not specified
	if callFrameID == "" {
		stack := e.session.state.GetCallStack()
		if len(stack) == 0 {
			return e.EvaluateGlobal(expression)
		}
		callFrameID = stack[0].ID
	}

	params := map[string]any{
		"callFrameId":           callFrameID,
		"expression":            expression,
		"generatePreview":       true,
		"includeCommandLineAPI": true,
		"returnByValue":         false,
	}

	result, err := e.session.Call("Debugger.evaluateOnCallFrame", params)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	return parseEvaluationResult(result)
}

// GetProperties gets the properties of an object
func (e *Evaluator) GetProperties(objectID string, ownProperties bool) ([]Property, error) {
	params := map[string]any{
		"objectId":               objectID,
		"ownProperties":          ownProperties,
		"accessorPropertiesOnly": false,
		"generatePreview":        true,
	}

	result, err := e.session.Call("Runtime.getProperties", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get properties: %w", err)
	}

	var response struct {
		Result []struct {
			Name         string         `json:"name"`
			Value        *RemoteObject  `json:"value,omitempty"`
			Writable     bool           `json:"writable"`
			Configurable bool           `json:"configurable"`
			Enumerable   bool           `json:"enumerable"`
			IsOwn        bool           `json:"isOwn"`
			Symbol       *RemoteObject  `json:"symbol,omitempty"`
		} `json:"result"`
		InternalProperties []struct {
			Name  string        `json:"name"`
			Value *RemoteObject `json:"value,omitempty"`
		} `json:"internalProperties"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	properties := make([]Property, 0, len(response.Result))
	for _, p := range response.Result {
		prop := Property{
			Name:         p.Name,
			Writable:     p.Writable,
			Configurable: p.Configurable,
			Enumerable:   p.Enumerable,
			IsOwn:        p.IsOwn,
		}
		if p.Value != nil {
			prop.Value = *p.Value
		}
		properties = append(properties, prop)
	}

	return properties, nil
}

// Property represents an object property
type Property struct {
	Name         string
	Value        RemoteObject
	Writable     bool
	Configurable bool
	Enumerable   bool
	IsOwn        bool
}

// GetScopeVariables gets variables in a scope
func (e *Evaluator) GetScopeVariables(scope Scope) ([]Property, error) {
	if scope.Object.ObjectID == "" {
		return nil, fmt.Errorf("scope has no object ID")
	}

	return e.GetProperties(scope.Object.ObjectID, true)
}

// GetLocalVariables gets local variables for the current frame
func (e *Evaluator) GetLocalVariables() ([]Property, error) {
	stack := e.session.state.GetCallStack()
	if len(stack) == 0 {
		return nil, fmt.Errorf("no call stack")
	}

	// Find the local scope
	for _, scope := range stack[0].ScopeChain {
		if scope.Type == "local" {
			return e.GetScopeVariables(scope)
		}
	}

	return nil, fmt.Errorf("no local scope found")
}

// ReleaseObject releases a remote object
func (e *Evaluator) ReleaseObject(objectID string) error {
	params := map[string]any{
		"objectId": objectID,
	}

	_, err := e.session.Call("Runtime.releaseObject", params)
	return err
}

// CallFunctionOn calls a function on a remote object
func (e *Evaluator) CallFunctionOn(objectID string, functionDeclaration string, args []any) (*EvaluationResult, error) {
	params := map[string]any{
		"objectId":            objectID,
		"functionDeclaration": functionDeclaration,
		"returnByValue":       false,
		"generatePreview":     true,
	}

	if len(args) > 0 {
		callArgs := make([]map[string]any, len(args))
		for i, arg := range args {
			callArgs[i] = map[string]any{"value": arg}
		}
		params["arguments"] = callArgs
	}

	result, err := e.session.Call("Runtime.callFunctionOn", params)
	if err != nil {
		return nil, fmt.Errorf("function call failed: %w", err)
	}

	return parseEvaluationResult(result)
}

// SetVariableValue sets the value of a variable
func (e *Evaluator) SetVariableValue(scopeNumber int, variableName string, newValue any, callFrameID string) error {
	if callFrameID == "" {
		stack := e.session.state.GetCallStack()
		if len(stack) == 0 {
			return fmt.Errorf("no call stack")
		}
		callFrameID = stack[0].ID
	}

	params := map[string]any{
		"scopeNumber":  scopeNumber,
		"variableName": variableName,
		"newValue":     map[string]any{"value": newValue},
		"callFrameId":  callFrameID,
	}

	_, err := e.session.Call("Debugger.setVariableValue", params)
	return err
}

// parseEvaluationResult parses the result of an evaluation
func parseEvaluationResult(result json.RawMessage) (*EvaluationResult, error) {
	var response struct {
		Result           map[string]any `json:"result"`
		ExceptionDetails *struct {
			ExceptionID  int            `json:"exceptionId"`
			Text         string         `json:"text"`
			LineNumber   int            `json:"lineNumber"`
			ColumnNumber int            `json:"columnNumber"`
			ScriptID     string         `json:"scriptId"`
			URL          string         `json:"url"`
			Exception    map[string]any `json:"exception,omitempty"`
		} `json:"exceptionDetails,omitempty"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	evalResult := &EvaluationResult{
		Result: parseRemoteObject(response.Result),
	}

	if response.ExceptionDetails != nil {
		ed := response.ExceptionDetails
		evalResult.ExceptionDetails = &ExceptionDetails{
			ExceptionID:  ed.ExceptionID,
			Text:         ed.Text,
			LineNumber:   ed.LineNumber,
			ColumnNumber: ed.ColumnNumber,
			ScriptID:     ed.ScriptID,
			URL:          ed.URL,
		}
		if ed.Exception != nil {
			exc := parseRemoteObject(ed.Exception)
			evalResult.ExceptionDetails.Exception = &exc
		}
	}

	return evalResult, nil
}

// FormatResult formats an evaluation result for display
func FormatResult(result *EvaluationResult) string {
	if result.ExceptionDetails != nil {
		return fmt.Sprintf("Error: %s", result.ExceptionDetails.Text)
	}

	obj := result.Result

	switch obj.Type {
	case "undefined":
		return "undefined"
	case "null":
		return "null"
	case "boolean", "number", "string":
		return fmt.Sprintf("%v", obj.Value)
	case "symbol", "bigint":
		return obj.Description
	case "function":
		if obj.Description != "" {
			return obj.Description
		}
		return "[Function]"
	case "object":
		if obj.Subtype != "" {
			if obj.Description != "" {
				return obj.Description
			}
			return fmt.Sprintf("[%s]", obj.Subtype)
		}
		if obj.ClassName != "" {
			return fmt.Sprintf("%s {...}", obj.ClassName)
		}
		return "{...}"
	default:
		if obj.Description != "" {
			return obj.Description
		}
		return fmt.Sprintf("[%s]", obj.Type)
	}
}
