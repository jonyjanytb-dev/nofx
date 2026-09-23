package kernel

import (
	"encoding/json"
	"fmt"
	"math"
)

// Models sometimes use a 0..1 fractional confidence even when the prompt
// requests 0..100. Normalize that descriptive field without changing the
// integer confidence contract used by the decision and execution layers.
func (d *Decision) UnmarshalJSON(data []byte) error {
	type plainDecision Decision
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["confidence"]; ok && string(raw) != "null" {
		var number float64
		if err := json.Unmarshal(raw, &number); err != nil {
			return fmt.Errorf("confidence must be numeric: %w", err)
		}
		if number < 0 || number > 100 {
			return fmt.Errorf("confidence must be between 0 and 100")
		}
		if number > 0 && number < 1 {
			number *= 100
		}
		fields["confidence"] = json.RawMessage(fmt.Sprintf("%.0f", math.Round(number)))
		var err error
		data, err = json.Marshal(fields)
		if err != nil {
			return err
		}
	}
	var parsed plainDecision
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*d = Decision(parsed)
	return nil
}
