package dnf

import "fmt"

const (
	guildInviteNotification uint16 = 147
	guildInviteReplyCommand uint16 = 155
	guildInviteAccept       byte   = 1
)

func (r *RobotVo) handleGuildPacketUnsafe(packet robotInboundPacket) {
	if packet.typ != guildInviteNotification || packet.flag != 0 || r.State != StateRun {
		return
	}

	guildNameSize, inviterNameSize, source, err := selectGuildInvitePacket(r.Cipher, packet.data, packet.isAnti)
	if err != nil {
		fmt.Printf("[GUILD_INVITE_PARSE_WARN] uid=%d err=%v anti=%t size=%d\n", r.UID, err, packet.isAnti, packet.size)
	}

	var body [16]byte
	body[0] = guildInviteAccept
	pkt, err := buildSendPacket(guildInviteReplyCommand, uint16(r.PacketID), body[:], r.Cipher)
	if err != nil {
		fmt.Printf("[GUILD_INVITE_ACCEPT_BUILD_ERROR] uid=%d err=%v\n", r.UID, err)
		return
	}
	if !r.sendRaw(pkt) {
		fmt.Printf("[GUILD_INVITE_ACCEPT_SEND_ERROR] uid=%d\n", r.UID)
		return
	}
	r.PacketID++
	// The invite packet has no guild ID. Mark membership as pending immediately;
	// reconnecting will replace this sentinel with the persistent database value.
	if r.GuildID == 0 {
		r.GuildID = -1
	}
	fmt.Printf("[GUILD_AUTO_ACCEPT] uid=%d guild_name_bytes=%d inviter_name_bytes=%d source=%s\n",
		r.UID, guildNameSize, inviterNameSize, source)
}
