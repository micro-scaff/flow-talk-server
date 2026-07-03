package models

import "testing"

func TestBuildDirectMembersSetsJoinedAt(t *testing.T) {
	members := buildDirectMembers(10, 1, 2)

	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}
	for _, member := range members {
		if member.JoinedAt.IsZero() {
			t.Fatalf("member %d JoinedAt is zero", member.UserID)
		}
	}
}

func TestBuildGroupMembersSetsJoinedAt(t *testing.T) {
	members := buildGroupMembers(10, 1, []int64{1, 2, 3})

	if len(members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(members))
	}
	for _, member := range members {
		if member.JoinedAt.IsZero() {
			t.Fatalf("member %d JoinedAt is zero", member.UserID)
		}
	}
}
