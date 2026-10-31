package hosts

import "testing"

const base = "127.0.0.1 localhost\n"
const shopBlock = "# lodev:shop begin\n127.0.0.1 a.test b.test\n# lodev:shop end\n"

func TestRenderAppend(t *testing.T) {
	if got := Render(base, "shop", []string{"a.test", "b.test"}); got != base+shopBlock {
		t.Fatalf("%q", got)
	}
}

func TestRenderNoTrailingNewline(t *testing.T) {
	if got := Render("127.0.0.1 localhost", "shop", []string{"a.test", "b.test"}); got != base+shopBlock {
		t.Fatalf("%q", got)
	}
}

func TestRenderReplace(t *testing.T) {
	other := "# lodev:blog begin\n127.0.0.1 blog.test\n# lodev:blog end\n"
	in := base + shopBlock + other + "10.0.0.1 nas\n"
	want := base + "# lodev:shop begin\n127.0.0.1 c.test\n# lodev:shop end\n" + other + "10.0.0.1 nas\n"
	if got := Render(in, "shop", []string{"c.test"}); got != want {
		t.Fatalf("%q", got)
	}
}

func TestRenderRemove(t *testing.T) {
	if got := Render(base+shopBlock, "shop", nil); got != base {
		t.Fatalf("%q", got)
	}
}

func TestRenderIdempotent(t *testing.T) {
	once := Render(base, "shop", []string{"a.test"})
	if twice := Render(once, "shop", []string{"a.test"}); twice != once {
		t.Fatalf("%q", twice)
	}
}

func TestRenderOtherBlockUntouched(t *testing.T) {
	shop2 := "# lodev:shop2 begin\n127.0.0.1 x.test\n# lodev:shop2 end\n"
	if got := Render(base+shop2, "shop", nil); got != base+shop2 {
		t.Fatalf("%q", got)
	}
}
