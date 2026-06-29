package conferences

import "testing"

func TestGroupByNetwork(t *testing.T) {
	confs := []*Conference{
		{ID: 1, Name: "General", Echo: false},
		{ID: 2, Name: "Fido", Echo: true, Network: "FidoNet", EchoTag: "FIDO"},
		{ID: 3, Name: "Lovely", Echo: true, Network: "LovlyNet", EchoTag: "LOVELY"},
		{ID: 4, Name: "PrimaryEcho", Echo: true, EchoTag: "PRIMARY"},
	}
	order := []string{"FidoNet", "LovlyNet"}
	groups := GroupByNetwork(confs, order, "FidoNet")
	if len(groups) != 3 {
		t.Fatalf("groups=%d want 3", len(groups))
	}
	if groups[0].Network != "" || len(groups[0].List) != 1 || groups[0].List[0].ID != 1 {
		t.Fatalf("local group: %+v", groups[0])
	}
	if groups[1].Network != "FidoNet" || len(groups[1].List) != 2 {
		t.Fatalf("fido group: %+v", groups[1])
	}
	if groups[1].List[0].ID != 2 || groups[1].List[1].ID != 4 {
		t.Fatalf("fido order: %+v", groups[1].List)
	}
	if groups[2].Network != "LovlyNet" || groups[2].List[0].ID != 3 {
		t.Fatalf("lovly group: %+v", groups[2])
	}
}

func TestEffectiveNetwork(t *testing.T) {
	local := &Conference{Echo: false, Network: "FidoNet"}
	if EffectiveNetwork(local, "FidoNet") != "" {
		t.Fatal("non-echo should be local")
	}
	echo := &Conference{Echo: true}
	if EffectiveNetwork(echo, "FidoNet") != "FidoNet" {
		t.Fatal("blank network should use primary")
	}
}
