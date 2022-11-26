package primitive

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestObjectID(t *testing.T) {
	id := "6382570e90a8f60e557fe4a1"
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		t.Errorf("creating test objectID: %v", err)
	}
	tests := []struct {
		name   string
		id     string
		wantOk bool
		want   *primitive.ObjectID
	}{
		{
			name: "invalid",
			id:   "deadbeef",
		},
		{
			name:   "happy path",
			id:     id,
			wantOk: true,
			want:   &objID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ObjectIDFromString(test.id)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted %v \n got:   %v", test.want, got)
			}
		})
	}
}

func TestObjectIDFrom(t *testing.T) {
	id := "6382570e90a8f60e557fe4a1"
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		t.Errorf("creating test objectID: %v", err)
	}
	tests := []struct {
		name   string
		id     interface{}
		wantOk bool
		want   *primitive.ObjectID
	}{
		{
			name: "not objectID",
			id:   id,
		},
		{
			name:   "happy path",
			id:     objID,
			wantOk: true,
			want:   &objID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ToObjectID(test.id)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted %v \n got:   %v", test.want, got)
			}
		})
	}
}

func TestMatchAnyIgnoreCaseRegex(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  primitive.Regex
	}{
		{"nil", nil, primitive.Regex{Options: "i"}},
		{"empty", []string{}, primitive.Regex{Options: "i"}},
		{"single", []string{"word"}, primitive.Regex{Pattern: "word", Options: "i"}},
		{"three", []string{"a", "b", "c"}, primitive.Regex{Pattern: "a|b|c", Options: "i"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, MatchAnyIgnoreCaseRegex(test.words...); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted %v \n got:   %v", test.want, got)
			}
		})
	}
}
