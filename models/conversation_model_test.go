package models

import "testing"

func TestConversationToDetailDTOCountsOnlyActiveMembers(t *testing.T) {
	conversation := Conversation{ID: 16, Type: ConversationTypeGroup}
	members := []ConversationMember{
		{UserID: 1, Role: MemberRoleOwner, Status: MemberStatusActive},
		{UserID: 2, Role: MemberRoleMember, Status: MemberStatusActive},
		{UserID: 3, Role: MemberRoleMember, Status: MemberStatusLeft},
		{UserID: 4, Role: MemberRoleMember, Status: MemberStatusRemoved},
	}

	detail := conversation.ToDetailDTO(members)

	if detail.MemberCount != 2 {
		t.Fatalf("ToDetailDTO().MemberCount = %d, want 2", detail.MemberCount)
	}
	if len(detail.Members) != len(members) {
		t.Fatalf("len(ToDetailDTO().Members) = %d, want %d", len(detail.Members), len(members))
	}
}
