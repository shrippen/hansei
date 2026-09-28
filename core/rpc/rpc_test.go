package rpc

import (
	"path/filepath"
	"testing"
	"time"

	"git.arianw.de/shrippen/hansei/core/service"
	"git.arianw.de/shrippen/hansei/core/testvault"
)

func TestRoundTrip(t *testing.T) {
	c := testvault.New(t)
	svc, err := service.Open(c, service.Options{DataDir: t.TempDir(), Lang: "de"})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	sock := filepath.Join(t.TempDir(), "h.sock")
	srv, err := Listen(svc, sock)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	defer srv.Close()

	cl, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	if st := cl.Status(); st.Notes != 7 || st.Lang != "de" {
		t.Fatalf("status %+v", st)
	}
	events, stop := cl.Subscribe()
	defer stop()

	sum, err := cl.FromFinding("codename", "")
	if err != nil || len(sum.Files) != 2 {
		t.Fatalf("fromFinding %v %+v", err, sum)
	}
	select {
	case e := <-events:
		if e.Kind != service.EventBatch {
			t.Errorf("event %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}

	fv, err := cl.File(sum.ID, sum.Files[0].Path, "split", 3, false)
	if err != nil || len(fv.Hunks) != 1 {
		t.Fatalf("file %v %+v", err, fv)
	}
	res, err := cl.Decide(sum.ID, fv.Path, fv.Hunks[0].ID, "accepted", "")
	if err != nil || !res.Written {
		t.Fatalf("decide %v %+v", err, res)
	}
	if _, err := cl.Batch("nope"); err == nil {
		t.Error("unknown batch without error")
	}
	if err := cl.call("nope", nil, nil); err == nil {
		t.Error("unknown method without error")
	}
}
