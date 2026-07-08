package schemaengine

import (
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ToDynamic converts an arbitrary Go value - as produced by yaml.v3
// unmarshaling into interface{} (map[string]interface{}, []interface{},
// string, int, float64, bool, nil) - into a terraform-plugin-framework
// types.Dynamic value suitable for a schema.DynamicAttribute.
func ToDynamic(v interface{}) (types.Dynamic, error) {
	val, err := toAttrValue(v)
	if err != nil {
		return types.Dynamic{}, err
	}
	return types.DynamicValue(val), nil
}

// toAttrValue recursively converts v into an attr.Value. Nested maps become
// Object values whose attributes are all typed as DynamicType (so mixed
// value types survive at any depth), and nested slices become List values
// of DynamicType elements for the same reason.
func toAttrValue(v interface{}) (attr.Value, error) {
	switch val := v.(type) {
	case nil:
		return types.DynamicNull(), nil
	case types.Dynamic:
		return val, nil
	case string:
		return types.StringValue(val), nil
	case bool:
		return types.BoolValue(val), nil
	case int:
		return types.NumberValue(big.NewFloat(float64(val))), nil
	case int64:
		return types.NumberValue(big.NewFloat(float64(val))), nil
	case float64:
		return types.NumberValue(big.NewFloat(val)), nil
	case map[string]interface{}:
		attrTypes := make(map[string]attr.Type, len(val))
		attrValues := make(map[string]attr.Value, len(val))
		for k, child := range val {
			childVal, err := toAttrValue(child)
			if err != nil {
				return nil, err
			}
			attrTypes[k] = types.DynamicType
			attrValues[k] = types.DynamicValue(childVal)
		}
		obj, diags := types.ObjectValue(attrTypes, attrValues)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to build object value: %s", diags)
		}
		return obj, nil
	case []interface{}:
		elems := make([]attr.Value, len(val))
		for i, child := range val {
			childVal, err := toAttrValue(child)
			if err != nil {
				return nil, err
			}
			elems[i] = types.DynamicValue(childVal)
		}
		list, diags := types.ListValue(types.DynamicType, elems)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to build list value: %s", diags)
		}
		return list, nil
	default:
		return nil, fmt.Errorf("unsupported type %T for dynamic conversion", v)
	}
}
