package dnf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"robot/internal/protocol/dnf/crypt"
)

func guildInviteTestBody(guildName, inviterName []byte) []byte {
	body := make([]byte, 8+len(guildName)+len(inviterName))
	binary.LittleEndian.PutUint32(body[0:4], uint32(len(guildName)))
	copy(body[4:], guildName)
	offset := 4 + len(guildName)
	binary.LittleEndian.PutUint32(body[offset:offset+4], uint32(len(inviterName)))
	copy(body[offset+4:], inviterName)
	return body
}

func paddedGuildInviteTestBody(guildName, inviterName []byte) []byte {
	body := guildInviteTestBody(guildName, inviterName)
	return append(body, make([]byte, alignTo16(len(body))-len(body))...)
}

func guildInviteInboundPacket(t *testing.T, cipher *crypt.DNFCipher, body []byte) []byte {
	t.Helper()
	encrypted, err := cipher.Encrypt(guildInviteNotification, body)
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 15+len(encrypted))
	binary.LittleEndian.PutUint16(packet[1:3], guildInviteNotification)
	binary.LittleEndian.PutUint32(packet[3:7], uint32(len(packet)))
	copy(packet[15:], encrypted)
	return packet
}

func TestParseGuildInvite(t *testing.T) {
	body := append(guildInviteTestBody([]byte("guild"), []byte("inviter")), make([]byte, 7)...)
	guildSize, inviterSize, ok := parseGuildInvite(body)
	if !ok || guildSize != 5 || inviterSize != 7 {
		t.Fatalf("parsed guild=%d inviter=%d ok=%t", guildSize, inviterSize, ok)
	}

	invalid := [][]byte{
		nil,
		guildInviteTestBody(nil, []byte("inviter")),
		guildInviteTestBody([]byte("guild"), nil),
		append(guildInviteTestBody([]byte("guild"), []byte("inviter")), make([]byte, 16)...),
	}
	for _, data := range invalid {
		if _, _, ok := parseGuildInvite(data); ok {
			t.Fatalf("invalid guild invite accepted: %x", data)
		}
	}
}

func TestMalformedGuildInviteStillAccepts(t *testing.T) {
	conn := &captureSessionConn{}
	cipher := crypt.NewDNFCipher()
	if err := cipher.Initialize(make([]byte, 334)); err != nil {
		t.Fatal(err)
	}

	robot := NewRobotVo(nil)
	robot.Cipher = cipher
	robot.Conn = conn
	robot.State = StateRun
	robot.PacketID = 12
	robot.handleGuildPacketUnsafe(robotInboundPacket{
		data: make([]byte, 15),
		size: 15,
		flag: 0,
		typ:  guildInviteNotification,
	})

	if len(conn.written) == 0 || robot.PacketID != 13 {
		t.Fatalf("malformed invite reply bytes=%d packet_id=%d", len(conn.written), robot.PacketID)
	}
}

func TestGuildInviteIsAutomaticallyAccepted(t *testing.T) {
	conn := &captureSessionConn{}
	cipher := crypt.NewDNFCipher()
	if err := cipher.Initialize(make([]byte, 334)); err != nil {
		t.Fatal(err)
	}
	body := paddedGuildInviteTestBody([]byte("guild"), []byte("inviter"))
	inbound := guildInviteInboundPacket(t, cipher, body)
	var reply [16]byte
	reply[0] = guildInviteAccept
	want, err := buildSendPacket(guildInviteReplyCommand, 41, reply[:], cipher)
	if err != nil {
		t.Fatal(err)
	}

	robot := NewRobotVo(nil)
	robot.UID = 17000001
	robot.Cipher = cipher
	robot.Conn = conn
	robot.State = StateRun
	robot.PacketID = 41
	robot.handleGuildPacketUnsafe(robotInboundPacket{
		data: inbound,
		size: len(inbound),
		flag: 0,
		typ:  guildInviteNotification,
	})

	if !bytes.Equal(conn.written, want) {
		t.Fatalf("guild accept packet = %x, want %x", conn.written, want)
	}
	if robot.PacketID != 42 {
		t.Fatalf("packet id = %d, want 42", robot.PacketID)
	}
	if robot.GuildID != -1 {
		t.Fatalf("pending guild id = %d, want -1", robot.GuildID)
	}
}

func TestGuildInviteSendFailureKeepsPacketSequence(t *testing.T) {
	conn := &captureSessionConn{writeErr: errors.New("write failed")}
	cipher := crypt.NewDNFCipher()
	if err := cipher.Initialize(make([]byte, 334)); err != nil {
		t.Fatal(err)
	}
	body := paddedGuildInviteTestBody([]byte("guild"), []byte("inviter"))
	inbound := guildInviteInboundPacket(t, cipher, body)

	robot := NewRobotVo(nil)
	robot.Cipher = cipher
	robot.Conn = conn
	robot.State = StateRun
	robot.PacketID = 41
	robot.handleGuildPacketUnsafe(robotInboundPacket{data: inbound, size: len(inbound), flag: 0, typ: guildInviteNotification})
	if robot.PacketID != 41 {
		t.Fatalf("packet id = %d, want 41 after failed send", robot.PacketID)
	}
	if robot.GuildID != 0 {
		t.Fatalf("failed accept changed guild id to %d", robot.GuildID)
	}
}
