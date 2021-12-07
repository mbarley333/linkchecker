package linkchecker_test

import (
	"bytes"
	"linkchecker"
	"testing"
)

func TestAddToTotal(t *testing.T) {
	t.Parallel()

	b := linkchecker.NewBar()

	// check progress
	want := 0
	got := b.TotalSteps

	if want != got {
		t.Fatalf("want: %d, got: %d", want, got)
	}

	b.Add()

	wantAdd := 1
	gotAdd := b.TotalSteps

	if wantAdd != gotAdd {
		t.Fatalf("want: %d, got: %d", wantAdd, gotAdd)
	}

}

func TestAddToCompleted(t *testing.T) {
	t.Parallel()

	b := linkchecker.NewBar()

	// check progress
	want := 0
	got := b.CompletedSteps

	if want != got {
		t.Fatalf("want: %d, got: %d", want, got)
	}

	b.Completed()

	wantAdd := 1
	gotAdd := b.CompletedSteps

	if wantAdd != gotAdd {
		t.Fatalf("want: %d, got: %d", wantAdd, gotAdd)
	}

}

func TestGetPercentComplete(t *testing.T) {
	t.Parallel()

	b := linkchecker.NewBar()

	b.TotalSteps = 100.0
	b.CompletedSteps = 20.0

	want := 20.0
	got := b.GetPercent()

	if want != got {
		t.Fatalf("want: %v, got: %v", want, got)
	}

}

func TestRender(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}

	b := linkchecker.NewBar(
		linkchecker.WithOutputBar(output),
	)

	b.TotalSteps = 100.0
	b.CompletedSteps = 20.0

	b.Render()

	want := "20.0% complete"
	got := output.String()

	if want != got {
		t.Fatalf("want: %q, got: %q", want, got)
	}

}
