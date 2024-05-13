package bulkerlib

import (
	"fmt"
	"github.com/jitsucom/bulker/bulkerlib/types"
	"github.com/jitsucom/bulker/jitsubase/jsoniter"
	types2 "github.com/jitsucom/bulker/jitsubase/types"
	"github.com/jitsucom/bulker/jitsubase/utils"
)

type StreamOption func(*StreamOptions)

var optionsRegistry = make(map[string]ParseableOption)

// Not used by bulker. Just added here to be treated as known options and don't print errors
var ignoredOptions = []string{"functions", "streams", "dataLayout", "events", "debugTill", "hosts", "schedule", "timezone", "storageKey", "tableNamePrefix", "multithreading"}

var (
	BatchSizeOption = ImplementationOption[int]{
		Key:          "batchSize",
		DefaultValue: 0,
		ParseFunc:    utils.ParseInt,
	}

	// BatchFrequencyOption frequency of running batches in minutes
	BatchFrequencyOption = ImplementationOption[float64]{
		Key:          "frequency",
		DefaultValue: 0,
		ParseFunc:    utils.ParseFloat,
	}

	RetryBatchSizeOption = ImplementationOption[int]{
		Key:          "retryBatchSize",
		DefaultValue: 0,
		ParseFunc:    utils.ParseInt,
	}
	// RetryFrequencyOption frequency of running retry consumer in minutes
	RetryFrequencyOption = ImplementationOption[float64]{
		Key:          "retryFrequency",
		DefaultValue: 0,
		ParseFunc:    utils.ParseFloat,
	}

	ModeOption = ImplementationOption[BulkMode]{Key: "mode", ParseFunc: func(serialized any) (BulkMode, error) {
		switch v := serialized.(type) {
		case string:
			if v == "stream" {
				return Stream, nil
			} else if v == "batch" {
				return Batch, nil
			} else {
				return Unknown, fmt.Errorf("unknown mode: %s", v)
			}
		default:
			return Unknown, fmt.Errorf("invalid value type of mode option: %T", v)
		}
	},
	}

	PrimaryKeyOption = ImplementationOption[types2.Set[string]]{
		Key:          "primaryKey",
		DefaultValue: types2.Set[string]{},
		AdvancedParseFunc: func(o *ImplementationOption[types2.Set[string]], serializedValue any) (StreamOption, error) {
			switch v := serializedValue.(type) {
			case []string:
				return withPrimaryKey(o, v...), nil
			case string:
				if v == "" {
					return func(options *StreamOptions) {}, nil
				}
				return withPrimaryKey(o, v), nil
			default:
				return nil, fmt.Errorf("failed to parse 'primaryKey' option: %v incorrect type: %T expected string or []string", v, v)
			}
		},
	}

	DeduplicateOption = ImplementationOption[bool]{
		Key:          "deduplicate",
		DefaultValue: false,
		ParseFunc:    utils.ParseBool,
	}

	PartitionIdOption = ImplementationOption[string]{
		Key:       "partitionId",
		ParseFunc: utils.ParseString,
	}

	// TimestampOption - field name that contains timestamp. For creating sorting indexes or partitions by that field in destination tables
	TimestampOption = ImplementationOption[string]{
		Key:       "timestampColumn",
		ParseFunc: utils.ParseString,
	}

	SchemaOption = ImplementationOption[types.Schema]{
		Key: "schema",
		ParseFunc: func(serialized any) (types.Schema, error) {
			switch v := serialized.(type) {
			case types.Schema:
				return v, nil
			case string:
				schema := types.Schema{}
				err := jsoniter.Unmarshal([]byte(v), &schema)
				if err != nil {
					return types.Schema{}, fmt.Errorf("failed to parse schema: %v", err)
				}
				return schema, nil
			default:
				return types.Schema{}, fmt.Errorf("invalid value type of schema option: %T", v)
			}
		},
	}
)

func init() {
	RegisterOption(&ModeOption)
	RegisterOption(&BatchSizeOption)
	RegisterOption(&BatchFrequencyOption)
	RegisterOption(&RetryFrequencyOption)
	RegisterOption(&RetryBatchSizeOption)
	RegisterOption(&PrimaryKeyOption)
	RegisterOption(&DeduplicateOption)
	RegisterOption(&PartitionIdOption)
	RegisterOption(&TimestampOption)
	RegisterOption(&SchemaOption)

	dummyParse := func(_ any) (any, error) { return nil, nil }
	for _, ignoredOption := range ignoredOptions {
		RegisterOption(&ImplementationOption[any]{Key: ignoredOption, ParseFunc: dummyParse})
	}

}

func RegisterOption[V any](option *ImplementationOption[V]) {
	optionsRegistry[option.Key] = option
}

func ParseOption(name string, serialized any) (StreamOption, error) {
	option, ok := optionsRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown option %s", name)
	}
	return option.Parse(serialized)

}

type StreamOptions struct {
	// Implementation options - map by option key. Values are parsed and validated
	// Don't access this map directly, use 'Get' method of specific option instance. E.g. `PartitionIdOption.Get(&so)`
	valuesMap map[string]any
	// options slice. To pass to CreateStream method
	Options []StreamOption
}

func (so *StreamOptions) Add(option StreamOption) {
	option(so)
	so.Options = append(so.Options, option)
}

type ParseableOption interface {
	Parse(serialized any) (StreamOption, error)
}

type ImplementationOption[V any] struct {
	Key               string
	DefaultValue      V
	ParseFunc         func(serialized any) (V, error)
	AdvancedParseFunc func(*ImplementationOption[V], any) (StreamOption, error)
}

func (io *ImplementationOption[V]) Parse(serializedValue any) (StreamOption, error) {
	if io.ParseFunc != nil {
		val, err := io.ParseFunc(serializedValue)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s' option: %v", io.Key, err)
		}
		return func(options *StreamOptions) {
			io.Set(options, val)
		}, nil
	} else {
		return io.AdvancedParseFunc(io, serializedValue)
	}
}

func (io *ImplementationOption[V]) Get(so *StreamOptions) V {
	opt, ok := so.valuesMap[io.Key].(V)
	if ok {
		return opt
	}
	return io.DefaultValue
}

func (io *ImplementationOption[V]) Set(so *StreamOptions, value V) {
	if so.valuesMap == nil {
		so.valuesMap = map[string]any{io.Key: value}
	} else {
		so.valuesMap[io.Key] = value
	}
}

func WithOption[T any](o *ImplementationOption[T], value T) StreamOption {
	return func(options *StreamOptions) {
		o.Set(options, value)
	}
}

func withPrimaryKey(o *ImplementationOption[types2.Set[string]], pkFields ...string) StreamOption {
	return func(options *StreamOptions) {
		set := o.Get(options)
		if len(set) == 0 {
			o.Set(options, types2.NewSet(pkFields...))
		} else {
			set.PutAll(pkFields)
		}
	}
}

func WithPrimaryKey(pkFields ...string) StreamOption {
	return withPrimaryKey(&PrimaryKeyOption, pkFields...)
}

// WithDeduplicate - when true merge rows on primary keys collision.
func WithDeduplicate() StreamOption {
	return WithOption(&DeduplicateOption, true)
}

// WithPartition settings for bulker.ReplacePartition mode only
// partitionId - value of `__partition_id`  for current BulkerStream e.g. id of current partition
// TODO: For bigquery require string in special format
func WithPartition(partitionId string) StreamOption {
	return WithOption(&PartitionIdOption, partitionId)
}

func WithTimestamp(timestampField string) StreamOption {
	return WithOption(&TimestampOption, timestampField)
}

func WithSchema(schema types.Schema) StreamOption {
	return WithOption(&SchemaOption, schema)
}
