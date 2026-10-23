package cmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestSenderFromMessage(t *testing.T) {
	message := "From: =?UTF-8?Q?Ada_Lovelace?= <ada@example.net>\r\nSubject: Test\r\n\r\nBody"
	sender, err := senderFromMessage(strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	if sender.Name != "Ada Lovelace" || sender.Address != "ada@example.net" {
		t.Fatalf("sender = %#v", sender)
	}
}

func TestSenderFromMessageRequiresFromHeader(t *testing.T) {
	_, err := senderFromMessage(strings.NewReader("Subject: Test\r\n\r\nBody"))
	if err == nil {
		t.Fatal("message without From header succeeded")
	}
}

func TestSenderFromMessageDecodesISO2022JPName(t *testing.T) {
	message := "From: =?ISO-2022-JP?B?GyRCOzNFREJATzobKEI=?= <taro@example.jp>\r\n\r\nBody"
	sender, err := senderFromMessage(strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	if sender.Name != "山田太郎" || sender.Address != "taro@example.jp" {
		t.Fatalf("sender = %#v", sender)
	}
}

func TestVersionCommand(t *testing.T) {
	oldVersion := version
	version = "v1.2.3"
	t.Cleanup(func() { version = oldVersion })

	command := newVersionCommand()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "go-khard v1.2.3 (" + runtime.Version() + ")\n"
	if stdout.String() != want {
		t.Fatalf("version output = %q, want %q", stdout.String(), want)
	}
}
