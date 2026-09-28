package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/service"
)

const dialTimeout = 2 * time.Second

// ErrClosed means the connection to the daemon is gone.
var ErrClosed = errors.New("rpc: connection to the Hansei daemon closed")

// Client implements service.API against a running daemon.
type Client struct {
	c       net.Conn
	w       *bufio.Writer
	wmu     sync.Mutex
	mu      sync.Mutex
	next    int
	pending map[int]chan response
	subs    map[int]chan service.Event
	subN    int
	closed  bool
}

var _ service.API = (*Client)(nil)

// Dial connects to the daemon socket.
func Dial(path string) (*Client, error) {
	nc, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return nil, err
	}
	cl := &Client{c: nc, w: bufio.NewWriter(nc), pending: map[int]chan response{}, subs: map[int]chan service.Event{}}
	go cl.read()
	return cl, nil
}

// Close ends the connection.
func (cl *Client) Close() error { return cl.c.Close() }

type wire struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (cl *Client) read() {
	sc := bufio.NewScanner(cl.c)
	sc.Buffer(make([]byte, 0, 1<<20), maxMessage)
	for sc.Scan() {
		var m wire
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m.Method == "event" {
			var e service.Event
			if json.Unmarshal(m.Params, &e) == nil {
				cl.fanout(e)
			}
			continue
		}
		if m.ID == nil {
			continue
		}
		cl.mu.Lock()
		ch, ok := cl.pending[*m.ID]
		delete(cl.pending, *m.ID)
		cl.mu.Unlock()
		if ok {
			ch <- response{Result: m.Result, Error: m.Error}
		}
	}

	// Connection gone: fail all waiting calls and close event channels.
	cl.mu.Lock()
	cl.closed = true
	for id, ch := range cl.pending {
		ch <- response{Error: &rpcError{Code: codeInternal, Message: ErrClosed.Error()}}
		delete(cl.pending, id)
	}
	for id, ch := range cl.subs {
		close(ch)
		delete(cl.subs, id)
	}
	cl.mu.Unlock()
}

func (cl *Client) fanout(e service.Event) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for _, ch := range cl.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// call sends a request and decodes the result into out (may be nil).
func (cl *Client) call(method string, params, out any) error {
	cl.mu.Lock()
	if cl.closed {
		cl.mu.Unlock()
		return ErrClosed
	}
	cl.next++
	id := cl.next
	ch := make(chan response, 1)
	cl.pending[id] = ch
	cl.mu.Unlock()

	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	cl.wmu.Lock()
	_, err = cl.w.Write(append(raw, '\n'))
	if err == nil {
		err = cl.w.Flush()
	}
	cl.wmu.Unlock()
	if err != nil {
		return err
	}

	res := <-ch
	if res.Error != nil {
		return errors.New(res.Error.Message)
	}
	if out == nil {
		return nil
	}
	data, ok := res.Result.(json.RawMessage)
	if !ok {
		return fmt.Errorf("rpc: unexpected result for %s", method)
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Subscribe asks the daemon for events.
func (cl *Client) Subscribe() (<-chan service.Event, func()) {
	ch := make(chan service.Event, 256)
	cl.mu.Lock()
	cl.subN++
	id := cl.subN
	cl.subs[id] = ch
	first := len(cl.subs) == 1
	cl.mu.Unlock()
	if first {
		_ = cl.call("subscribe", nil, nil)
	}
	return ch, func() {
		cl.mu.Lock()
		defer cl.mu.Unlock()
		if c, ok := cl.subs[id]; ok {
			close(c)
			delete(cl.subs, id)
		}
	}
}

// get is call for methods whose errors the API does not return.
func get[T any](cl *Client, method string, params any) T {
	var out T
	_ = cl.call(method, params, &out)
	return out
}

func fetch[T any](cl *Client, method string, params any) (T, error) {
	var out T
	err := cl.call(method, params, &out)
	return out, err
}

func (cl *Client) Status() service.StatusView { return get[service.StatusView](cl, "status", nil) }
func (cl *Client) Home() service.HomeView     { return get[service.HomeView](cl, "home", nil) }
func (cl *Client) Findings(refresh bool) service.FindingsView {
	return get[service.FindingsView](cl, "findings", map[string]bool{"refresh": refresh})
}
func (cl *Client) Batches() []service.BatchSummary {
	return get[[]service.BatchSummary](cl, "batches", nil)
}
func (cl *Client) Batch(id string) (service.BatchView, error) {
	return fetch[service.BatchView](cl, "batch", idParams{id})
}
func (cl *Client) File(id, path, mode string, context int, reveal bool) (service.FileView, error) {
	return fetch[service.FileView](cl, "file", fileParams{ID: id, Path: path, Mode: mode, Context: context, Reveal: reveal})
}
func (cl *Client) Compare(id, path string, from, to int, mode string, context int) (service.CompareView, error) {
	return fetch[service.CompareView](cl, "compare", fileParams{ID: id, Path: path, From: from, To: to, Mode: mode, Context: context})
}
func (cl *Client) Decide(id, path, hunk, decision, reason string) (service.DecideResult, error) {
	return fetch[service.DecideResult](cl, "decide", fileParams{ID: id, Path: path, Hunk: hunk, Decision: decision, Reason: reason})
}
func (cl *Client) DecideFile(id, path, decision string) (service.DecideResult, error) {
	return fetch[service.DecideResult](cl, "decideFile", fileParams{ID: id, Path: path, Decision: decision})
}
func (cl *Client) EditHunk(id, path, hunk, text string) (service.FileSummary, error) {
	return fetch[service.FileSummary](cl, "editHunk", fileParams{ID: id, Path: path, Hunk: hunk, Text: text})
}
func (cl *Client) EditFile(id, path, content string) (service.FileSummary, error) {
	return fetch[service.FileSummary](cl, "editFile", fileParams{ID: id, Path: path, Content: content})
}
func (cl *Client) SetVersion(id, path string, n int) (service.FileSummary, error) {
	return fetch[service.FileSummary](cl, "setVersion", fileParams{ID: id, Path: path, N: n})
}
func (cl *Client) Reopen(id, path string) (service.FileSummary, error) {
	return fetch[service.FileSummary](cl, "reopen", fileParams{ID: id, Path: path})
}
func (cl *Client) UndoFile(id, path string) (service.FileSummary, error) {
	return fetch[service.FileSummary](cl, "undoFile", fileParams{ID: id, Path: path})
}
func (cl *Client) UndoBatch(id string) (service.BatchSummary, error) {
	return fetch[service.BatchSummary](cl, "undoBatch", idParams{id})
}
func (cl *Client) Journal(limit int) []service.JournalView {
	return get[[]service.JournalView](cl, "journal", map[string]int{"limit": limit})
}
func (cl *Client) Discard(id string) error { return cl.call("discard", idParams{id}, nil) }
func (cl *Client) Cancel(id string)        { _ = cl.call("cancel", idParams{id}, nil) }
func (cl *Client) Task(in service.TaskInput) (service.BatchSummary, error) {
	return fetch[service.BatchSummary](cl, "task", in)
}
func (cl *Client) Estimate(in service.TaskInput) (service.Estimate, error) {
	return fetch[service.Estimate](cl, "estimate", in)
}
func (cl *Client) Feedback(in service.FeedbackInput) (batch.Message, error) {
	return fetch[batch.Message](cl, "feedback", in)
}
func (cl *Client) Regenerate(id, path, hunk string) (batch.Message, error) {
	return fetch[batch.Message](cl, "regenerate", fileParams{ID: id, Path: path, Hunk: hunk})
}
func (cl *Client) Suggestion(id, sid string, accept bool) (service.BatchSummary, error) {
	return fetch[service.BatchSummary](cl, "suggestion", map[string]any{"id": id, "sid": sid, "accept": accept})
}
func (cl *Client) FromFinding(rule, provider string) (service.BatchSummary, error) {
	return fetch[service.BatchSummary](cl, "fromFinding", map[string]string{"rule": rule, "provider": provider})
}
func (cl *Client) Import() (int, error) {
	out, err := fetch[map[string]int](cl, "import", nil)
	return out["imported"], err
}
func (cl *Client) Settings() service.SettingsView {
	return get[service.SettingsView](cl, "settings", nil)
}
func (cl *Client) SetSettings(p service.SettingsPatch) (service.SettingsView, error) {
	return fetch[service.SettingsView](cl, "setSettings", p)
}
func (cl *Client) SetKey(provider, key string) error {
	return cl.call("setKey", map[string]string{"provider": provider, "key": key}, nil)
}
func (cl *Client) CheckProvider(name string) service.ProviderCheck {
	return get[service.ProviderCheck](cl, "checkProvider", map[string]string{"name": name})
}
func (cl *Client) RuleSection(ref string) (service.RuleSectionView, error) {
	return fetch[service.RuleSectionView](cl, "ruleSection", map[string]string{"ref": ref})
}
func (cl *Client) JournalDiff(id string) (service.CompareView, error) {
	return fetch[service.CompareView](cl, "journalDiff", idParams{id})
}
