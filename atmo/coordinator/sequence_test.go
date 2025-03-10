package coordinator

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/suborbital/atmo/atmo/appsource"
	"github.com/suborbital/atmo/atmo/options"
	"github.com/suborbital/atmo/directive"
	"github.com/suborbital/reactr/request"
	"github.com/suborbital/vektor/vk"
	"github.com/suborbital/vektor/vlog"
)

var coord *Coordinator

func init() {
	opts := options.NewWithModifiers(
		options.UseLogger(vlog.Default(
			vlog.Level(vlog.LogLevelDebug),
		)),
	)

	appSource := appsource.NewBundleSource("../../example-project/runnables.wasm.zip")

	coord = New(appSource, opts)

	if err := coord.Start(); err != nil {
		opts.Logger.Error(errors.Wrap(err, "failed to coord.Start"))
	}
}

func TestBasicSequence(t *testing.T) {
	steps := []directive.Executable{
		{
			CallableFn: directive.CallableFn{
				Fn:   "helloworld-rs",
				FQFN: "com.suborbital.test#default::helloworld-rs@v0.0.1",
			},
		},
	}

	seq := newSequence(steps, coord.exec, vk.NewCtx(coord.log, nil, nil))

	req := &request.CoordinatedRequest{
		Method: "GET",
		URL:    "/hello/world",
		ID:     uuid.New().String(),
		Body:   []byte("world"),
		State:  map[string][]byte{},
	}

	state, err := seq.execute(req)
	if err != nil {
		t.Error(err)
		return
	}

	if val, ok := state.state["helloworld-rs"]; !ok {
		t.Error("helloworld state is missing")
	} else if !bytes.Equal(val, []byte("hello world")) {
		t.Error("unexpected helloworld state value:", string(val))
	}
}

func TestGroupSequence(t *testing.T) {
	steps := []directive.Executable{
		{
			Group: []directive.CallableFn{
				{
					Fn:   "helloworld-rs",
					FQFN: "com.suborbital.test#default::helloworld-rs@v0.0.1",
				},
				{
					Fn:   "get-file",
					FQFN: "com.suborbital.test#default::get-file@v0.0.1",
					As:   "main.md",
				},
			},
		},
	}

	seq := newSequence(steps, coord.exec, vk.NewCtx(coord.log, nil, nil))

	req := &request.CoordinatedRequest{
		Method: "GET",
		URL:    "/hello/world",
		ID:     uuid.New().String(),
		Body:   []byte("world"),
		State: map[string][]byte{
			"file": []byte("main.md"),
		},
	}

	state, err := seq.execute(req)
	if err != nil {
		t.Error(err)
	}

	if val, ok := state.state["helloworld-rs"]; !ok {
		t.Error("helloworld state is missing")
	} else if !bytes.Equal(val, []byte("hello world")) {
		t.Error("unexpected helloworld state value:", string(val))
	}

	if val, ok := state.state["main.md"]; !ok {
		t.Error("get-file state is missing")
	} else if !bytes.Equal(val, []byte("## hello")) {
		t.Error("unexpected get-file state value:", string(val))
	}
}

func TestAsOnErrContinueSequence(t *testing.T) {
	steps := []directive.Executable{
		{
			CallableFn: directive.CallableFn{
				Fn:   "helloworld-rs",
				FQFN: "com.suborbital.test#default::helloworld-rs@v0.0.1",
				As:   "hello",
			},
		},
		{
			CallableFn: directive.CallableFn{
				Fn:   "return-err",
				FQFN: "com.suborbital.test#default::return-err@v0.0.1",
				OnErr: &directive.FnOnErr{
					Any: "continue",
				},
			},
		},
	}

	seq := newSequence(steps, coord.exec, vk.NewCtx(coord.log, nil, nil))

	req := &request.CoordinatedRequest{
		Method: "GET",
		URL:    "/hello/world",
		ID:     uuid.New().String(),
		Body:   []byte("world"),
		State:  map[string][]byte{},
	}

	state, err := seq.execute(req)
	if err != nil {
		t.Error(err)
	}

	if val, ok := state.state["hello"]; !ok {
		t.Error("hello state is missing")
	} else if !bytes.Equal(val, []byte("hello world")) {
		t.Error("unexpected hello state value:", string(val))
	}
}

func TestAsOnErrReturnSequence(t *testing.T) {
	steps := []directive.Executable{
		{
			CallableFn: directive.CallableFn{
				Fn:   "helloworld-rs",
				FQFN: "com.suborbital.test#default::helloworld-rs@v0.0.1",
				As:   "hello",
			},
		},
		{
			CallableFn: directive.CallableFn{
				Fn:   "return-err",
				FQFN: "com.suborbital.test#default::return-err@v0.0.1",
				OnErr: &directive.FnOnErr{
					Any: "return",
				},
			},
		},
	}

	seq := newSequence(steps, coord.exec, vk.NewCtx(coord.log, nil, nil))

	req := &request.CoordinatedRequest{
		Method: "GET",
		URL:    "/hello/world",
		ID:     uuid.New().String(),
		Body:   []byte("world"),
		State:  map[string][]byte{},
	}

	state, err := seq.execute(req)
	if err != ErrSequenceRunErr {
		t.Error(errors.New("sequence should have returned ErrSequenceRunErr, did not"))
	}

	if state.err.Code != 400 {
		t.Error("error code should be 400, is actually", state.err.Code)
	}

	if state.err.Message != "job failed" {
		t.Error("message should be 'job failed', is actually", state.err.Message)
	}
}

func TestWithSequence(t *testing.T) {
	steps := []directive.Executable{
		{
			CallableFn: directive.CallableFn{
				Fn:   "helloworld-rs", // the body is empty, so this will return only "hello"
				FQFN: "com.suborbital.test#default::helloworld-rs@v0.0.1",
			},
		},
		{
			CallableFn: directive.CallableFn{
				Fn:   "modify-url", // if there's no body, it'll look in state for '
				FQFN: "com.suborbital.test#default::modify-url@v0.0.1",
				With: map[string]string{"url": "helloworld-rs"},
			},
		},
	}

	seq := newSequence(steps, coord.exec, vk.NewCtx(coord.log, nil, nil))

	req := &request.CoordinatedRequest{
		Method: "GET",
		URL:    "/hello/world",
		ID:     uuid.New().String(),
		Body:   []byte(""),
		State:  map[string][]byte{},
	}

	state, err := seq.execute(req)
	if err != nil {
		t.Error(err)
	}

	if val, ok := state.state["helloworld-rs"]; !ok {
		t.Error("helloworld-rs state is missing")
	} else if !bytes.Equal(val, []byte("hello ")) {
		t.Error("unexpected helloworld-rs state value:", string(val))
	}

	if val, ok := state.state["modify-url"]; !ok {
		t.Error("modify-url state is missing")
	} else if !bytes.Equal(val, []byte("hello /suborbital")) {
		t.Error("unexpected modify-url state value:", string(val))
	}
}
