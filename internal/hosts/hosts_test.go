package hosts

import "testing"

func render(t *testing.T, content, name string, domains []string) string {
	t.Helper()
	out, err := Render(content, name, domains)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const base = "127.0.0.1 localhost\n"
const shopBlock = "# lodev:shop begin\n127.0.0.1 a.test b.test\n# lodev:shop end\n"

func TestRenderAppend(t *testing.T) {
	if got := render(t, base, "shop", []string{"a.test", "b.test"}); got != base+shopBlock {
		t.Fatalf("%q", got)
	}
}

func TestRenderNoTrailingNewline(t *testing.T) {
	if got := render(t, "127.0.0.1 localhost", "shop", []string{"a.test", "b.test"}); got != base+shopBlock {
		t.Fatalf("%q", got)
	}
}

func TestRenderReplace(t *testing.T) {
	other := "# lodev:blog begin\n127.0.0.1 blog.test\n# lodev:blog end\n"
	in := base + shopBlock + other + "10.0.0.1 nas\n"
	want := base + "# lodev:shop begin\n127.0.0.1 c.test\n# lodev:shop end\n" + other + "10.0.0.1 nas\n"
	if got := render(t, in, "shop", []string{"c.test"}); got != want {
		t.Fatalf("%q", got)
	}
}

func TestRenderRemove(t *testing.T) {
	if got := render(t, base+shopBlock, "shop", nil); got != base {
		t.Fatalf("%q", got)
	}
}

func TestRenderIdempotent(t *testing.T) {
	once := render(t, base, "shop", []string{"a.test"})
	if twice := render(t, once, "shop", []string{"a.test"}); twice != once {
		t.Fatalf("%q", twice)
	}
}

func TestRenderOtherBlockUntouched(t *testing.T) {
	shop2 := "# lodev:shop2 begin\n127.0.0.1 x.test\n# lodev:shop2 end\n"
	if got := render(t, base+shop2, "shop", nil); got != base+shop2 {
		t.Fatalf("%q", got)
	}
}

func TestRenderRefusesUnterminatedBlock(t *testing.T) {
	// A hand-deleted end marker must not swallow the rest of /etc/hosts.
	in := base + "# lodev:shop begin\n127.0.0.1 a.test\n10.0.0.1 nas\n"
	if _, err := Render(in, "shop", nil); err == nil {
		t.Fatal("unterminated block accepted")
	}
}
