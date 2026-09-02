package multirotate

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/blang/semver"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
)

var testNow = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func checkedInputs(count, periodDays float64) resource.PropertyMap {
	return resource.PropertyMap{
		"count":              resource.NewNumberProperty(count),
		"rotationPeriodDays": resource.NewNumberProperty(periodDays),
	}
}

func TestCheckDefaultsAndValidation(t *testing.T) {
	p := NewMultiRotateProvider()
	ctx := context.Background()

	t.Run("defaults are applied", func(t *testing.T) {
		res, err := p.Check(ctx, plugin.CheckRequest{News: resource.PropertyMap{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := res.Properties["count"].NumberValue(); got != 1 {
			t.Errorf("count = %v, want 1", got)
		}
		if got := res.Properties["rotationPeriodDays"].NumberValue(); got != 60 {
			t.Errorf("rotationPeriodDays = %v, want 60", got)
		}
		if len(res.Failures) != 0 {
			t.Errorf("failures = %v, want none", res.Failures)
		}
	})

	t.Run("valid values pass through", func(t *testing.T) {
		res, err := p.Check(ctx, plugin.CheckRequest{News: checkedInputs(2, 30)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := res.Properties["count"].NumberValue(); got != 2 {
			t.Errorf("count = %v, want 2", got)
		}
		if len(res.Failures) != 0 {
			t.Errorf("failures = %v, want none", res.Failures)
		}
	})

	t.Run("non-positive count fails", func(t *testing.T) {
		for _, c := range []float64{0, -3} {
			res, err := p.Check(ctx, plugin.CheckRequest{News: checkedInputs(c, 60)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res.Failures) != 1 || res.Failures[0].Property != "count" || res.Failures[0].Reason != "Must be a positive integer" {
				t.Errorf("failures = %v, want single count failure", res.Failures)
			}
		}
	})

	t.Run("non-integer values fail", func(t *testing.T) {
		res, err := p.Check(ctx, plugin.CheckRequest{News: checkedInputs(1.5, 60.5)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Failures) != 2 {
			t.Fatalf("failures = %v, want 2", res.Failures)
		}
		if res.Failures[0].Property != "count" || res.Failures[1].Property != "rotationPeriodDays" {
			t.Errorf("failures = %v, want count then rotationPeriodDays", res.Failures)
		}
	})
}

func TestCreate(t *testing.T) {
	p := NewMultiRotateProvider()
	res, err := p.Create(context.Background(), plugin.CreateRequest{Properties: checkedInputs(2, 60)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID == "" {
		t.Fatal("expected a non-empty id")
	}
	_, ok := parseISO(string(res.ID))
	if !ok {
		t.Errorf("id %q is not a valid timestamp", res.ID)
	}
	got := stringArray(res.Properties, "timestamps")
	if len(got) != 2 || got[0] != string(res.ID) || got[1] != string(res.ID) {
		t.Errorf("timestamps = %v, want 2 copies of %q", got, res.ID)
	}
	if got := res.Properties["index"].NumberValue(); got != 0 {
		t.Errorf("index = %v, want 0", got)
	}
	if res.Properties["currentTimestamp"].StringValue() != string(res.ID) {
		t.Errorf("currentTimestamp = %v, want %q", res.Properties["currentTimestamp"].StringValue(), res.ID)
	}
	if got := res.Properties["count"].NumberValue(); got != 2 {
		t.Errorf("count = %v, want 2", got)
	}
	if got := res.Properties["rotationPeriodDays"].NumberValue(); got != 60 {
		t.Errorf("rotationPeriodDays = %v, want 60", got)
	}
	if res.Status != resource.StatusOK {
		t.Errorf("status = %v, want ok", res.Status)
	}
}

func stateWith(timestamps []string, index int, count, periodDays float64) resource.PropertyMap {
	props := make([]resource.PropertyValue, len(timestamps))
	for i, ts := range timestamps {
		props[i] = resource.NewStringProperty(ts)
	}
	return resource.PropertyMap{
		"count":              resource.NewNumberProperty(count),
		"rotationPeriodDays": resource.NewNumberProperty(periodDays),
		"index":              resource.NewNumberProperty(float64(index)),
		"timestamps":         resource.NewArrayProperty(props),
		"currentTimestamp":   resource.NewStringProperty(timestamps[index]),
	}
}

func TestDiffRotation(t *testing.T) {
	mk := func(daysAgo float64) string {
		return formatISO(testNow.Add(-time.Duration(daysAgo) * 24 * time.Hour))
	}

	t.Run("fresh state, no changes", func(t *testing.T) {
		old := stateWith([]string{mk(10), mk(20)}, 0, 2, 60)
		if got := diffRotation(testNow, old, checkedInputs(2, 60)); got != plugin.DiffNone {
			t.Errorf("got %v, want DiffNone", got)
		}
	})

	t.Run("expired timestamp at cursor", func(t *testing.T) {
		old := stateWith([]string{mk(61), mk(10)}, 0, 2, 60)
		if got := diffRotation(testNow, old, checkedInputs(2, 60)); got != plugin.DiffSome {
			t.Errorf("got %v, want DiffSome", got)
		}
	})

	t.Run("cursor points at fresh slot", func(t *testing.T) {
		old := stateWith([]string{mk(100), mk(10)}, 1, 2, 60)
		if got := diffRotation(testNow, old, checkedInputs(2, 60)); got != plugin.DiffNone {
			t.Errorf("got %v, want DiffNone", got)
		}
	})

	t.Run("count mismatch", func(t *testing.T) {
		old := stateWith([]string{mk(10)}, 0, 1, 60)
		if got := diffRotation(testNow, old, checkedInputs(2, 60)); got != plugin.DiffSome {
			t.Errorf("got %v, want DiffSome", got)
		}
	})

	t.Run("incomplete state", func(t *testing.T) {
		old := resource.PropertyMap{
			"index": resource.NewNumberProperty(0),
		}
		if got := diffRotation(testNow, old, checkedInputs(1, 60)); got != plugin.DiffSome {
			t.Errorf("got %v, want DiffSome", got)
		}
	})

	t.Run("period change alone does not change", func(t *testing.T) {
		old := stateWith([]string{mk(10)}, 0, 1, 60)
		if got := diffRotation(testNow, old, checkedInputs(1, 90)); got != plugin.DiffNone {
			// 10 days < 90 days, so the slot is still fresh under the new period.
			t.Errorf("got %v, want DiffNone", got)
		}
	})
}

func TestUpdateRotation(t *testing.T) {
	mk := func(daysAgo float64) string {
		return formatISO(testNow.Add(-time.Duration(daysAgo) * 24 * time.Hour))
	}

	t.Run("not expired: state unchanged", func(t *testing.T) {
		old := stateWith([]string{mk(10), mk(20)}, 0, 2, 60)
		got := updateRotation(testNow, old, checkedInputs(2, 60))
		if s := stringArray(got, "timestamps"); len(s) != 2 || s[0] != mk(10) || s[1] != mk(20) {
			t.Errorf("timestamps = %v, want unchanged", s)
		}
		if got["index"].NumberValue() != 0 {
			t.Errorf("index = %v, want 0", got["index"].NumberValue())
		}
		if got["currentTimestamp"].StringValue() != mk(10) {
			t.Errorf("currentTimestamp = %v, want %v", got["currentTimestamp"].StringValue(), mk(10))
		}
	})

	t.Run("expired: cursor advances and slot re-stamped", func(t *testing.T) {
		old := stateWith([]string{mk(10), mk(61)}, 1, 2, 60)
		got := updateRotation(testNow, old, checkedInputs(2, 60))
		s := stringArray(got, "timestamps")
		if got["index"].NumberValue() != 0 {
			t.Errorf("index = %v, want 0 (wrapped from 1)", got["index"].NumberValue())
		}
		// The cursor was at slot 1 (expired); it advances to slot 0 and re-stamps
		// the slot it moved to, leaving the previous slot untouched.
		if s[0] != formatISO(testNow) {
			t.Errorf("timestamps[0] = %v, want current time (re-stamped)", s[0])
		}
		if s[1] != mk(61) {
			t.Errorf("timestamps[1] = %v, want %v (previous slot untouched)", s[1], mk(61))
		}
		if got["currentTimestamp"].StringValue() != formatISO(testNow) {
			t.Errorf("currentTimestamp = %v, want current time", got["currentTimestamp"].StringValue())
		}
	})

	t.Run("growing count appends fresh slots", func(t *testing.T) {
		old := stateWith([]string{mk(10)}, 0, 1, 60)
		got := updateRotation(testNow, old, checkedInputs(2, 60))
		s := stringArray(got, "timestamps")
		if len(s) != 2 {
			t.Fatalf("timestamps = %v, want length 2", s)
		}
		if s[1] != formatISO(testNow) {
			t.Errorf("timestamps[1] = %v, want current time", s[1])
		}
	})

	t.Run("shrinking count truncates and wraps index", func(t *testing.T) {
		old := stateWith([]string{mk(10), mk(20), mk(30)}, 2, 3, 60)
		got := updateRotation(testNow, old, checkedInputs(1, 60))
		s := stringArray(got, "timestamps")
		if len(s) != 1 || s[0] != mk(10) {
			t.Errorf("timestamps = %v, want [t0]", s)
		}
		if got["index"].NumberValue() != 0 {
			t.Errorf("index = %v, want 0 (2 %% 1)", got["index"].NumberValue())
		}
	})

	t.Run("period is stored from news", func(t *testing.T) {
		old := stateWith([]string{mk(10)}, 0, 1, 60)
		got := updateRotation(testNow, old, checkedInputs(1, 90))
		if got["rotationPeriodDays"].NumberValue() != 90 {
			t.Errorf("rotationPeriodDays = %v, want 90", got["rotationPeriodDays"].NumberValue())
		}
	})
}

func TestRead(t *testing.T) {
	p := NewMultiRotateProvider()
	ctx := context.Background()

	state := stateWith([]string{"2026-01-01T00:00:00.000Z"}, 0, 1, 60)

	t.Run("returns stored state", func(t *testing.T) {
		res, err := p.Read(ctx, plugin.ReadRequest{ID: "id1", Inputs: checkedInputs(1, 60), State: state})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ID != "id1" {
			t.Errorf("id = %v, want id1", res.ID)
		}
		if len(res.Outputs) == 0 {
			t.Error("expected outputs to be returned")
		}
	})

	t.Run("missing resource yields empty result", func(t *testing.T) {
		res, err := p.Read(ctx, plugin.ReadRequest{ID: "id1", Inputs: checkedInputs(1, 60)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ID != "" || len(res.Outputs) != 0 {
			t.Errorf("expected empty result, got %+v", res)
		}
	})
}

func TestDeleteAndConfigure(t *testing.T) {
	p := NewMultiRotateProvider()
	ctx := context.Background()

	if res, err := p.Delete(ctx, plugin.DeleteRequest{}); err != nil || res.Status != 0 {
		t.Errorf("Delete = (%+v, %v), want no-op", res, err)
	}
	if _, err := p.Configure(ctx, plugin.ConfigureRequest{}); err != nil {
		t.Errorf("Configure: unexpected error %v", err)
	}
	if res, err := p.CheckConfig(ctx, plugin.CheckConfigRequest{News: resource.PropertyMap{"k": resource.NewStringProperty("v")}}); err != nil || res.Properties["k"].StringValue() != "v" {
		t.Errorf("CheckConfig = (%+v, %v), want news echoed", res, err)
	}
	if res, err := p.DiffConfig(ctx, plugin.DiffConfigRequest{}); err != nil || res.Changes != plugin.DiffNone {
		t.Errorf("DiffConfig = (%+v, %v), want DiffNone", res, err)
	}
}

func TestGetPluginInfoAndSchema(t *testing.T) {
	p := NewMultiRotateProvider()
	ctx := context.Background()

	want, err := semver.ParseTolerant(version)
	if err != nil {
		t.Fatalf("build-time version %q is not semver: %v", version, err)
	}

	info, err := p.GetPluginInfo(ctx)
	if err != nil || info.Version == nil || info.Version.String() != want.String() {
		t.Errorf("GetPluginInfo = (%+v, %v), want %s", info, err, want)
	}

	res, err := p.GetSchema(ctx, plugin.GetSchemaRequest{})
	if err != nil || len(res.Schema) == 0 {
		t.Fatalf("GetSchema = (%+v, %v), want embedded schema", res, err)
	}

	// The served schema must advertise the same version as the plugin, so that
	// generated SDKs pin the exact plugin they were generated from.
	var doc struct {
		Name              string `json:"name"`
		Version           string `json:"version"`
		PluginDownloadURL string `json:"pluginDownloadURL"`
	}
	if err := json.Unmarshal(res.Schema, &doc); err != nil {
		t.Fatalf("GetSchema returned invalid JSON: %v", err)
	}
	if doc.Name != "multirotate" {
		t.Errorf("schema name = %q, want %q", doc.Name, "multirotate")
	}
	if doc.Version != want.String() {
		t.Errorf("schema version = %q, want %q", doc.Version, want.String())
	}
	if doc.PluginDownloadURL == "" {
		t.Error("schema pluginDownloadURL is empty; the plugin would not be auto-installable")
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close: unexpected error %v", err)
	}
}
