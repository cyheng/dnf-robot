package dnf

import "testing"

func TestSetAreaRejectsGenericGuildAgitDestinationForGuildMember(t *testing.T) {
	robot := NewRobotVo(nil)
	robot.State = StateRun
	robot.GuildID = 2
	robot.PacketID = 41

	robot.SetArea(8, 2, 622, 488)

	if robot.PacketID != 41 {
		t.Fatalf("packet id=%d want=41; unsafe CMD 38 was built", robot.PacketID)
	}
	if robot.CurVillage == 8 {
		t.Fatal("unsafe destination was committed to runtime state")
	}
}
