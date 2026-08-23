package config

import "testing"

func TestParseNodeInvitePassword(t *testing.T) {
	n, err := parseNode(`millo:https://discord.gg/mjS5J2K3ep@lava-v4.millohost.my.id:443?secure`)
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "millo" {
		t.Errorf("name=%q", n.Name)
	}
	if n.Password != "https://discord.gg/mjS5J2K3ep" {
		t.Errorf("password=%q", n.Password)
	}
	if n.Address != "lava-v4.millohost.my.id:443" {
		t.Errorf("address=%q", n.Address)
	}
	if !n.Secure {
		t.Error("secure should be true")
	}
}

func TestParseNodePlain(t *testing.T) {
	n, err := parseNode("main:youshallnotpass@localhost:2333")
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "main" || n.Password != "youshallnotpass" || n.Address != "localhost:2333" || n.Secure {
		t.Errorf("got %+v", n)
	}
}

func TestParseNodeWssPrefix(t *testing.T) {
	n, err := parseNode("backup:mypass@wss://free.lavalink.dev:443")
	if err != nil {
		t.Fatal(err)
	}
	if !n.Secure || n.Address != "free.lavalink.dev:443" {
		t.Errorf("got %+v", n)
	}
}
