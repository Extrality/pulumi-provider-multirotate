package multirotate

import (
	"context"
	_ "embed"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/blang/semver"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
)

//go:embed schema.json
var schemaJSON []byte

// version is the provider's semantic version. It defaults to 0.0.1 and is
// overridden at build time via -ldflags "-X .../version=<version>" so that the
// compiled binary self-reports the version it was released as.
var version = "0.0.1"

// versionedSchema returns the embedded schema with its "version" field
// rewritten to the build-time version, so a single git tag drives both the
// plugin version and the version baked into every generated SDK.
var versionedSchema = sync.OnceValue(func() []byte {
	var doc map[string]any
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return schemaJSON
	}
	v, err := semver.ParseTolerant(version)
	if err != nil {
		return schemaJSON
	}
	doc["version"] = v.String()
	out, err := json.Marshal(doc)
	if err != nil {
		return schemaJSON
	}
	return out
})

// timeLayout matches the output of JS `Date.prototype.toISOString`
// (millisecond precision, UTC, "Z" suffix).
const timeLayout = "2006-01-02T15:04:05.000Z"

const (
	defaultCount      = 1
	defaultPeriodDays = 60
)

// MultiRotateProvider is a Pulumi resource provider that rotates a cursor
// over a fixed number of timestamps on a time basis. Each timestamp expires
// after `rotationPeriodDays` days, at which point the cursor advances to the
// next slot and that slot is re-stamped with the current time.
type MultiRotateProvider struct {
	plugin.UnimplementedProvider
}

var _ plugin.Provider = (*MultiRotateProvider)(nil)

func NewMultiRotateProvider() *MultiRotateProvider {
	return &MultiRotateProvider{}
}

func (p *MultiRotateProvider) Close() error { return nil }

func (p *MultiRotateProvider) GetPluginInfo(context.Context) (plugin.PluginInfo, error) {
	v, err := semver.ParseTolerant(version)
	if err != nil {
		// A bad build-time version should never fail the plugin; fall back to the default.
		v = semver.Version{Major: 0, Minor: 0, Patch: 1}
	}
	return plugin.PluginInfo{Version: &v}, nil
}

func (p *MultiRotateProvider) GetSchema(context.Context, plugin.GetSchemaRequest) (plugin.GetSchemaResponse, error) {
	return plugin.GetSchemaResponse{Schema: versionedSchema()}, nil
}

func (p *MultiRotateProvider) CheckConfig(_ context.Context, req plugin.CheckConfigRequest) (plugin.CheckConfigResponse, error) {
	return plugin.CheckConfigResponse{Properties: req.News}, nil
}

func (p *MultiRotateProvider) DiffConfig(context.Context, plugin.DiffConfigRequest) (plugin.DiffConfigResponse, error) {
	return plugin.DiffResult{Changes: plugin.DiffNone}, nil
}

func (p *MultiRotateProvider) Configure(context.Context, plugin.ConfigureRequest) (plugin.ConfigureResponse, error) {
	return plugin.ConfigureResponse{}, nil
}

// Check normalizes and validates the new inputs, mirroring the previous
// dynamic provider: `count` defaults to 1 and `rotationPeriodDays` defaults
// to 60, and both must be positive integers.
func (p *MultiRotateProvider) Check(_ context.Context, req plugin.CheckRequest) (plugin.CheckResponse, error) {
	out := req.News.Copy()
	failures := []plugin.CheckFailure{}

	if !hasNumber(out, "count") {
		out["count"] = resource.NewNumberProperty(defaultCount)
	} else if !positiveInteger(out["count"]) {
		failures = append(failures, plugin.CheckFailure{
			Property: "count",
			Reason:   "Must be a positive integer",
		})
	}

	if !hasNumber(out, "rotationPeriodDays") {
		out["rotationPeriodDays"] = resource.NewNumberProperty(defaultPeriodDays)
	} else if !positiveInteger(out["rotationPeriodDays"]) {
		failures = append(failures, plugin.CheckFailure{
			Property: "rotationPeriodDays",
			Reason:   "Must be a positive integer",
		})
	}

	return plugin.CheckResponse{Properties: out, Failures: failures}, nil
}

// Create stamps every slot with the current time, mirroring the previous
// dynamic provider: the id is the current timestamp and the initial cursor
// is at index 0.
func (p *MultiRotateProvider) Create(_ context.Context, req plugin.CreateRequest) (plugin.CreateResponse, error) {
	now := time.Now().UTC()
	cur := formatISO(now)

	count, period := normalizedInputs(req.Properties)
	timestamps := make([]string, count)
	for i := range timestamps {
		timestamps[i] = cur
	}

	return plugin.CreateResponse{
		ID:         resource.ID(cur),
		Properties: rotationState(count, period, 0, timestamps, timestamps[0]),
		Status:     resource.StatusOK,
	}, nil
}

// Read returns the stored state unchanged; there is no external truth to
// reconcile with.
func (p *MultiRotateProvider) Read(_ context.Context, req plugin.ReadRequest) (plugin.ReadResponse, error) {
	if len(req.State) == 0 {
		return plugin.ReadResponse{}, nil
	}
	return plugin.ReadResponse{
		ReadResult: plugin.ReadResult{
			ID:      req.ID,
			Inputs:  req.Inputs,
			Outputs: req.State,
		},
		Status: resource.StatusOK,
	}, nil
}

// Diff reports a change when the stored state is incomplete, the timestamp
// count no longer matches, or the timestamp at the cursor has expired.
func (p *MultiRotateProvider) Diff(_ context.Context, req plugin.DiffRequest) (plugin.DiffResponse, error) {
	old := mergeMaps(req.OldInputs, req.OldOutputs)
	changes := diffRotation(time.Now().UTC(), old, req.NewInputs)
	return plugin.DiffResult{Changes: changes}, nil
}

// Update advances the rotation cursor when the timestamp at the cursor has
// expired, mirroring the previous dynamic provider's update semantics.
func (p *MultiRotateProvider) Update(_ context.Context, req plugin.UpdateRequest) (plugin.UpdateResponse, error) {
	now := time.Now().UTC()
	old := mergeMaps(req.OldInputs, req.OldOutputs)
	state := updateRotation(now, old, req.NewInputs)
	return plugin.UpdateResponse{Properties: state, Status: resource.StatusOK}, nil
}

func (p *MultiRotateProvider) Delete(context.Context, plugin.DeleteRequest) (plugin.DeleteResponse, error) {
	return plugin.DeleteResponse{}, nil
}

// --- pure logic (exported for testing) ---

// formatISO formats t in the JS toISOString shape.
func formatISO(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

// parseISO parses a timestamp produced by formatISO, tolerating the
// millisecond part being absent.
func parseISO(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// hasNumber reports whether m[key] is a number.
func hasNumber(m resource.PropertyMap, key resource.PropertyKey) bool {
	v, ok := m[key]
	return ok && v.IsNumber()
}

// positiveInteger reports whether v is a number holding a positive integer.
func positiveInteger(v resource.PropertyValue) bool {
	if !v.IsNumber() {
		return false
	}
	n := v.NumberValue()
	return n >= 1 && n == math.Trunc(n)
}

// normalizedInputs extracts count and rotationPeriodDays from checked
// inputs, applying the provider defaults if they are missing.
func normalizedInputs(m resource.PropertyMap) (count int, periodDays float64) {
	count = defaultCount
	if v, ok := m["count"]; ok && v.IsNumber() {
		count = int(v.NumberValue())
	}
	if count < 1 {
		count = 1
	}
	periodDays = defaultPeriodDays
	if v, ok := m["rotationPeriodDays"]; ok && v.IsNumber() {
		periodDays = v.NumberValue()
	}
	if periodDays < 1 {
		periodDays = defaultPeriodDays
	}
	return count, periodDays
}

// hasKeys reports whether all keys are present in m.
func hasKeys(m resource.PropertyMap, keys ...resource.PropertyKey) bool {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// intNumber reads m[key] as an int, returning def if it is missing or not a number.
func intNumber(m resource.PropertyMap, key resource.PropertyKey, def int) int {
	if v, ok := m[key]; ok && v.IsNumber() {
		return int(v.NumberValue())
	}
	return def
}

// stringArray reads m[key] as a string array, returning nil if it is missing or not an array.
func stringArray(m resource.PropertyMap, key resource.PropertyKey) []string {
	v, ok := m[key]
	if !ok || !v.IsArray() {
		return nil
	}
	var out []string
	for _, e := range v.ArrayValue() {
		if e.IsString() {
			out = append(out, e.StringValue())
		}
	}
	return out
}

// mergeMaps returns base with overlay values winning on conflicts.
func mergeMaps(base, overlay resource.PropertyMap) resource.PropertyMap {
	out := resource.PropertyMap{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// rotationInterval converts days to a time duration.
func rotationInterval(periodDays float64) time.Duration {
	return time.Duration(int64(periodDays)) * 24 * time.Hour
}

// rotationState builds the full recorded state for a rotation resource.
func rotationState(count int, periodDays float64, index int, timestamps []string, currentTs string) resource.PropertyMap {
	props := make([]resource.PropertyValue, len(timestamps))
	for i, ts := range timestamps {
		props[i] = resource.NewStringProperty(ts)
	}
	return resource.PropertyMap{
		"count":              resource.NewNumberProperty(float64(count)),
		"rotationPeriodDays": resource.NewNumberProperty(periodDays),
		"index":              resource.NewNumberProperty(float64(index)),
		"timestamps":         resource.NewArrayProperty(props),
		"currentTimestamp":   resource.NewStringProperty(currentTs),
	}
}

// expiry returns the time at which ts stops being valid, given the rotation
// period in days.
func expiry(ts string, periodDays float64) (time.Time, bool) {
	t, ok := parseISO(ts)
	if !ok {
		return time.Time{}, false
	}
	return t.Add(rotationInterval(periodDays)), true
}

// diffRotation mirrors the previous dynamic provider's diff: it reports a
// change when the recorded state is incomplete, the timestamp count no
// longer matches the new `count`, or the timestamp at the cursor has
// expired.
func diffRotation(now time.Time, old, news resource.PropertyMap) plugin.DiffChanges {
	count, period := normalizedInputs(news)

	if !hasKeys(old, "index", "timestamps", "currentTimestamp", "rotationPeriodDays") {
		return plugin.DiffSome
	}

	timestamps := stringArray(old, "timestamps")
	if len(timestamps) != count {
		return plugin.DiffSome
	}

	index := intNumber(old, "index", 0)
	if index < 0 || index >= len(timestamps) {
		return plugin.DiffSome
	}

	exp, ok := expiry(timestamps[index], period)
	if !ok || now.After(exp) {
		return plugin.DiffSome
	}
	return plugin.DiffNone
}

// updateRotation mirrors the previous dynamic provider's update: it grows
// or shrinks the timestamp list to match `count`, then advances the cursor
// and re-stamps that slot if the timestamp at the cursor has expired.
func updateRotation(now time.Time, old, news resource.PropertyMap) resource.PropertyMap {
	count, period := normalizedInputs(news)
	cur := formatISO(now)

	timestamps := stringArray(old, "timestamps")
	index := intNumber(old, "index", 0)

	for len(timestamps) < count {
		timestamps = append(timestamps, cur)
	}
	if len(timestamps) > count {
		timestamps = timestamps[:count]
		if count > 0 {
			index %= count
		}
	}
	if len(timestamps) == 0 {
		// Unreachable: check guarantees count >= 1, but stay defensive.
		index = 0
		timestamps = []string{cur}
		count = 1
	} else if index < 0 || index >= len(timestamps) {
		index %= len(timestamps)
	}

	if exp, ok := expiry(timestamps[index], period); ok && now.After(exp) {
		index = (index + 1) % len(timestamps)
		timestamps[index] = cur
	}

	return rotationState(count, period, index, timestamps, timestamps[index])
}
