// Tests migrated from OraHub MRs.

package ttc

import (
	"testing"
)

func TestParsePlaceholders_IgnoreTriggerPseudoRecords(t *testing.T) {
	t.Parallel()
	q := `CREATE OR REPLACE TRIGGER fk_trigger_members_refer_profiles_member_id
AFTER UPDATE OF refer ON members
FOR EACH ROW
BEGIN
  UPDATE profiles
  SET member_id = :NEW.refer
  WHERE member_id = :OLD.refer;
END;`

	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ph.bindNames) != 0 {
		t.Fatalf("expected trigger pseudo-record references to be ignored, got binds %v", ph.bindNames)
	}

	out, err := extractInputBindValues(ph, nil)
	if err != nil {
		t.Fatalf("expected extractInputBindValues to accept trigger DDL without args, got %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no ordered bind values, got %v", out)
	}
}
