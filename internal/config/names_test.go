package config

import "testing"

func TestDockerNames(t *testing.T) {
	p := &Project{Name: "shop"}
	for got, want := range map[string]string{
		p.ComposeProject():                      "lodev-shop",
		p.WebContainer():                        "lodev-shop-web",
		p.VarnishContainer():                    "lodev-shop-varnish",
		WebContainer("blog"):                    "lodev-blog-web",
		ServiceContainer("mysql"):               "lodev-mysql",
		ProjectOfWebContainer("lodev-shop-web"): "shop",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
