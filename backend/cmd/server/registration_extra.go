package main

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

// handleMembershipApply lets an already authenticated account claim a roster
// seat in another group. Approval only creates the group relationship; it
// never creates a second password or duplicate global account.
func (a *app) handleMembershipApply(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	if !a.requestLimiter.allow("membership-apply:"+clientIP(r)+":"+strings.ToLower(u.Username), 5, 24*time.Hour) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		GroupID       uint64 `json:"group_id"`
		Name          string `json:"name"`
		TermsAccepted bool   `json:"terms_accepted"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !req.TermsAccepted {
		writeError(w, http.StatusBadRequest, "terms_required")
		return
	}
	if req.GroupID == 0 || containsGroup(u.Groups, req.GroupID) {
		writeError(w, http.StatusConflict, "already_group_member")
		return
	}
	canonical, _ := canonicalRosterName(req.Name)
	if canonical == "" {
		writeError(w, http.StatusBadRequest, "roster_not_found")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "membership_apply_failed")
		return
	}
	defer tx.Rollback()
	var rosterID uint64
	var claimed sql.NullInt64
	err = tx.QueryRow(`SELECT e.id,e.claimed_by_user_id
		FROM roster_entries e JOIN study_groups g ON g.id=e.group_id
		WHERE e.group_id=? AND e.identity_key=? AND e.status=1 AND g.status=1 FOR UPDATE`,
		req.GroupID, identityKey(canonical)).Scan(&rosterID, &claimed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "roster_not_found")
		return
	}
	if claimed.Valid && uint64(claimed.Int64) != u.ID {
		writeError(w, http.StatusConflict, "roster_already_claimed")
		return
	}
	var exists int
	_ = tx.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id=? AND user_id=? AND status=1", req.GroupID, u.ID).Scan(&exists)
	if exists > 0 {
		writeError(w, http.StatusConflict, "already_group_member")
		return
	}
	_ = tx.QueryRow("SELECT COUNT(*) FROM registration_requests WHERE roster_entry_id=? AND status='pending'", rosterID).Scan(&exists)
	if exists > 0 {
		writeError(w, http.StatusConflict, "registration_pending")
		return
	}
	now := nowSQL()
	res, err := tx.Exec(`INSERT INTO registration_requests
		(roster_entry_id,group_id,applicant_user_id,request_kind,canonical_name,username,password_hash,status,terms_accepted_at,created_at,updated_at)
		VALUES(?,?,?,'join_group',?,?,?,'pending',?,?,?)`, rosterID, req.GroupID, u.ID, canonical, u.Username, "", now, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "membership_apply_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "membership_apply_failed")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusAccepted, map[string]any{"request_id": id, "status": "pending", "request_kind": "join_group"})
}
