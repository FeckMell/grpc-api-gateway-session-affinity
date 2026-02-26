package helpers

import (
	"context"
	"errors"
	"testing"

	"mygateway/interfaces/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// passthrough returns the same metadata.
type passthrough struct{}

func (passthrough) Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
	return headers.Copy(), nil
}

// addHeader adds a fixed key-value to metadata.
type addHeader struct {
	key, val string
}

func (a addHeader) Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
	out := headers.Copy()
	out.Set(a.key, a.val)
	return out, nil
}

// failProcessor returns an error.
type failProcessor struct{ err error }

func (f failProcessor) Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
	return nil, f.err
}

func TestHeaderProcessorChain_Passthrough(t *testing.T) {
	chain := HeaderProcessorChain{passthrough{}}
	md := metadata.Pairs("a", "1", "b", "2")
	out, err := chain.Process(context.Background(), md, "/svc/Method")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, []string{"1"}, out.Get("a"))
	assert.Equal(t, []string{"2"}, out.Get("b"))
}

func TestHeaderProcessorChain_AddHeader(t *testing.T) {
	chain := HeaderProcessorChain{passthrough{}, addHeader{key: "x-custom", val: "v"}}
	md := metadata.Pairs("a", "1")
	out, err := chain.Process(context.Background(), md, "/svc/Method")
	require.NoError(t, err)
	assert.Equal(t, []string{"1"}, out.Get("a"))
	assert.Equal(t, []string{"v"}, out.Get("x-custom"))
}

func TestHeaderProcessorChain_ErrorStopsPipeline(t *testing.T) {
	wantErr := errors.New("processor failed")
	chain := HeaderProcessorChain{
		addHeader{key: "first", val: "ok"},
		failProcessor{err: wantErr},
		addHeader{key: "second", val: "never"},
	}
	md := metadata.MD{}
	_, err := chain.Process(context.Background(), md, "/svc/Method")
	require.Error(t, err)
	assert.Equal(t, wantErr, err)
}

func TestHeaderProcessorChain_EmptyChain(t *testing.T) {
	chain := HeaderProcessorChain(nil)
	md := metadata.Pairs("k", "v")
	out, err := chain.Process(context.Background(), md, "/svc/Method")
	require.NoError(t, err)
	assert.Equal(t, []string{"v"}, out.Get("k"))
}

func TestNewHeaderProcessorChain_PanicsOnNilProcessor(t *testing.T) {
	assert.PanicsWithValue(t, "helpers.header_chain.go: processor at index 1 is required", func() {
		NewHeaderProcessorChain(passthrough{}, nil)
	})
}

func TestNewHeaderProcessorChain_PanicsOnNilProcessorsSlice(t *testing.T) {
	// Calling with no args passes nil slice to NilPanic.
	assert.PanicsWithValue(t, "helpers.header_chain.go: processors is required", func() {
		NewHeaderProcessorChain()
	})
}

func TestNewHeaderProcessorChain_ProcessWithMock(t *testing.T) {
	md := metadata.Pairs("x", "a", "y", "b")
	proc := &mock.HeaderProcessorMock{
		ProcessFunc: func(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
			out := headers.Copy()
			out.Set("processed", "true")
			return out, nil
		},
	}
	chain := NewHeaderProcessorChain(proc)
	out, err := chain.Process(context.Background(), md, "/svc/Method")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, []string{"a"}, out.Get("x"))
	assert.Equal(t, []string{"b"}, out.Get("y"))
	assert.Equal(t, []string{"true"}, out.Get("processed"))
}
