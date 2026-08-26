package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type groupOptions struct {
	SiteTitle          string `json:"site_title"`
	HomeMessage        string `json:"home_message"`
	RetroDays          int    `json:"retro_days"`
	ShowGroupSummary   bool   `json:"show_group_summary"`
	ShowMemberStatus   bool   `json:"show_member_status"`
	ShowReflections    bool   `json:"show_reflections"`
	AllowMemberRanking bool   `json:"allow_member_ranking"`
}

func defaultGroupOptions() groupOptions {
	return groupOptions{RetroDays: 30, ShowGroupSummary: true}
}

func groupOptionsFromSettings(settings map[string]any) groupOptions {
	result := defaultGroupOptions()
	raw, ok := settings["group_options"].(map[string]any)
	if !ok {
		return result
	}
	result.SiteTitle = asString(raw["site_title"])
	result.HomeMessage = asString(raw["home_message"])
	if value, ok := raw["retro_days"].(float64); ok {
		result.RetroDays = clampInt(int(value), 0, 90)
	}
	result.ShowGroupSummary, _ = raw["show_group_summary"].(bool)
	result.ShowMemberStatus, _ = raw["show_member_status"].(bool)
	result.ShowReflections, _ = raw["show_reflections"].(bool)
	result.AllowMemberRanking, _ = raw["allow_member_ranking"].(bool)
	return result
}

func (a *app) loadGroupOptions(groupID uint64) groupOptions {
	settings, err := a.groupLearningConfig(groupID)
	if err != nil {
		return defaultGroupOptions()
	}
	return groupOptionsFromSettings(settings)
}

func (a *app) handleAdminGroupSettings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var name, description string
	if err := a.db.QueryRow("SELECT name,description FROM study_groups WHERE id=? AND status=1", groupID).Scan(&name, &description); err != nil {
		writeError(w, http.StatusNotFound, "group_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": map[string]any{"id": groupID, "name": name, "description": description}, "options": a.loadGroupOptions(groupID)})
}

func (a *app) handleAdminSaveGroupSettings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Options     groupOptions `json:"options"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || len([]rune(req.Name)) > 128 || len([]rune(req.Description)) > 512 || req.Options.RetroDays < 0 || req.Options.RetroDays > 90 {
		writeError(w, http.StatusBadRequest, "group_settings_invalid")
		return
	}
	settings, err := a.groupLearningConfig(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_settings_save_failed")
		return
	}
	settings["group_options"] = map[string]any{
		"site_title": req.Options.SiteTitle, "home_message": req.Options.HomeMessage,
		"retro_days": req.Options.RetroDays, "show_group_summary": req.Options.ShowGroupSummary,
		"show_member_status": req.Options.ShowMemberStatus, "show_reflections": req.Options.ShowReflections,
		"allow_member_ranking": req.Options.AllowMemberRanking,
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_settings_save_failed")
		return
	}
	defer tx.Rollback()
	if u.IsSuperAdmin || hasRole(u.Roles, roleGroupLeader) {
		if _, err := tx.Exec("UPDATE study_groups SET name=?,description=?,updated_at=? WHERE id=?", req.Name, req.Description, nowSQL(), groupID); err != nil {
			writeError(w, http.StatusInternalServerError, "group_settings_save_failed")
			return
		}
	}
	if err := upsertGroupLearningConfigTx(tx, groupID, settings); err != nil || tx.Commit() != nil {
		writeError(w, http.StatusInternalServerError, "group_settings_save_failed")
		return
	}
	a.audit(groupID, u.ID, "save_group_settings", "study_groups", groupID, nil, map[string]any{"name": req.Name, "options": req.Options}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleAdminCompletionStats(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	from := queryDate(r, "from", time.Now().In(a.location).AddDate(0, 0, -6))
	to := queryDate(r, "to", time.Now().In(a.location))
	fromDate, _ := time.ParseInLocation("2006-01-02", from, a.location)
	toDate, _ := time.ParseInLocation("2006-01-02", to, a.location)
	if toDate.Before(fromDate) || toDate.Sub(fromDate) > 93*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date_range_invalid")
		return
	}
	members, err := a.listMembers(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "completion_stats_failed")
		return
	}
	type counts struct{ Devotion, Book, Video, Verse, Total int }
	byUser := map[uint64]*counts{}
	rows, err := a.db.Query(`SELECT user_id,task_type,COUNT(*) FROM checkin_records
		WHERE group_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL
		GROUP BY user_id,task_type`, groupID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "completion_stats_failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var userID uint64
		var taskType string
		var count int
		if rows.Scan(&userID, &taskType, &count) != nil {
			continue
		}
		item := byUser[userID]
		if item == nil {
			item = &counts{}
			byUser[userID] = item
		}
		switch taskType {
		case "daily_devotion":
			item.Devotion = count
		case "weekly_book":
			item.Book = count
		case "weekly_video":
			item.Video = count
		case "weekly_verse":
			item.Verse = count
		}
		item.Total += count
	}
	items := make([]map[string]any, 0, len(members))
	for _, member := range members {
		userID, _ := member["user_id"].(uint64)
		item := byUser[userID]
		if item == nil {
			item = &counts{}
		}
		items = append(items, map[string]any{"user_id": userID, "member_name": member["member_name"], "username": member["username"], "daily_devotion": item.Devotion, "weekly_book": item.Book, "weekly_video": item.Video, "weekly_verse": item.Verse, "total": item.Total})
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "items": items})
}

func (a *app) handleAdminCreateCheckin(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		UserID      uint64 `json:"user_id"`
		TaskType    string `json:"task_type"`
		LogicalDate string `json:"logical_date"`
		Detail      string `json:"detail"`
		Note        string `json:"note"`
		Part        string `json:"part"`
		WeekID      uint64 `json:"week_id"`
		TaskID      uint64 `json:"task_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.TaskType = strings.ToLower(strings.TrimSpace(req.TaskType))
	logicalDate, err := time.ParseInLocation("2006-01-02", req.LogicalDate, a.location)
	if req.UserID == 0 || !validCheckinTaskType(req.TaskType) || err != nil || logicalDate.After(time.Now().In(a.location)) {
		writeError(w, http.StatusBadRequest, "checkin_invalid")
		return
	}
	var member int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id=? AND user_id=? AND status=1", groupID, req.UserID).Scan(&member); err != nil || member != 1 {
		writeError(w, http.StatusBadRequest, "member_not_found")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checkin_save_failed")
		return
	}
	defer tx.Rollback()
	now := nowSQL()
	res, err := tx.Exec(`INSERT INTO checkin_records
		(group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,created_by,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, groupID, req.UserID, nullableID(req.TaskID), nullableID(req.WeekID), req.LogicalDate, now, req.TaskType, "done", req.LogicalDate != time.Now().In(a.location).Format("2006-01-02"), truncate(req.Detail, 1000), truncate(req.Note, 4000), truncate(req.Part, 64), "admin", u.ID, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "checkin_save_failed")
		return
	}
	id, _ := res.LastInsertId()
	if err := insertReflectionTx(tx, groupID, req.UserID, uint64(id), req.LogicalDate, req.Note, now); err != nil || tx.Commit() != nil {
		writeError(w, http.StatusInternalServerError, "checkin_save_failed")
		return
	}
	a.audit(groupID, u.ID, "create_checkin", "checkin_records", uint64(id), nil, map[string]any{"user_id": req.UserID, "logical_date": req.LogicalDate, "task_type": req.TaskType}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (a *app) handleListReflections(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	limit := clampInt(queryInt(r, "page_size", 50), 1, 100)
	options := a.loadGroupOptions(groupID)
	where := "rf.group_id=? AND rf.deleted_at IS NULL AND (rf.user_id=? OR rf.visibility='group')"
	if !options.ShowReflections {
		where = "rf.group_id=? AND rf.deleted_at IS NULL AND rf.user_id=?"
	}
	rows, err := a.db.Query(`SELECT rf.id,rf.user_id,u.display_name,rf.logical_date,rf.content,rf.visibility,rf.created_at
		FROM reflections rf JOIN users u ON u.id=rf.user_id WHERE `+where+` ORDER BY rf.logical_date DESC,rf.id DESC LIMIT ?`, groupID, u.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reflections_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, userID uint64
		var name, content, visibility string
		var logicalDate, createdAt time.Time
		if rows.Scan(&id, &userID, &name, &logicalDate, &content, &visibility, &createdAt) == nil {
			items = append(items, map[string]any{"id": id, "user_id": userID, "display_name": name, "logical_date": logicalDate.Format("2006-01-02"), "content": content, "visibility": visibility, "created_at": createdAt.Format(time.RFC3339)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "group_sharing_enabled": options.ShowReflections})
}

func (a *app) resetUserPassword(w http.ResponseWriter, r *http.Request, requireMembership bool) {
	u := mustUser(r)
	userID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if userID == 0 || userID == u.ID {
		writeError(w, http.StatusBadRequest, "user_reset_invalid")
		return
	}
	if !a.requestLimiter.allow("password-reset:"+strconv.FormatUint(u.ID, 10)+":"+strconv.FormatUint(userID, 10), 10, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	groupID := u.CurrentGroupID
	if requireMembership {
		groupID = requireGroupID(w, u)
		if groupID == 0 {
			return
		}
		var member int
		if err := a.db.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id=? AND user_id=? AND status=1", groupID, userID).Scan(&member); err != nil || member != 1 {
			writeError(w, http.StatusNotFound, "member_not_found")
			return
		}
	}
	password := randomPassword(12)
	hash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	res, err := a.db.Exec("UPDATE users SET password_hash=?,must_change_password=1,auth_version=auth_version+1,updated_at=? WHERE id=? AND status=1 AND is_super_admin=0", hash, nowSQL(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reset_failed")
		return
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		writeError(w, http.StatusConflict, "user_reset_invalid")
		return
	}
	a.audit(groupID, u.ID, "reset_user_password", "users", userID, nil, map[string]any{"must_change_password": true}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "temporary_password": password})
}

func (a *app) handleAdminResetUserPassword(w http.ResponseWriter, r *http.Request) {
	a.resetUserPassword(w, r, true)
}
func (a *app) handleSuperResetUserPassword(w http.ResponseWriter, r *http.Request) {
	a.resetUserPassword(w, r, false)
}

func (a *app) handleSuperOverview(w http.ResponseWriter, r *http.Request) {
	var groups, members, users, todayCheckins int64
	_ = a.db.QueryRow("SELECT COUNT(*) FROM study_groups WHERE status=1").Scan(&groups)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM group_members WHERE status=1").Scan(&members)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM users WHERE status=1").Scan(&users)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM checkin_records WHERE logical_date=? AND deleted_at IS NULL", time.Now().In(a.location).Format("2006-01-02")).Scan(&todayCheckins)
	var storageBytes int64
	_ = filepath.WalkDir(a.assetsRoot, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil {
			storageBytes += info.Size()
		}
		return nil
	})
	backupStatus := map[string]any{"ok": false, "message": "尚无备份状态"}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(a.assetsRoot), "backups", "status.json")); err == nil {
		_ = json.Unmarshal(data, &backupStatus)
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups, "memberships": members, "users": users, "today_checkins": todayCheckins, "asset_bytes": storageBytes, "backup": backupStatus})
}

func (a *app) handleSuperSetGroupStatus(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var req struct {
		Status int `json:"status"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if groupID == 0 || (req.Status != 0 && req.Status != 1) {
		writeError(w, http.StatusBadRequest, "group_status_invalid")
		return
	}
	res, err := a.db.Exec("UPDATE study_groups SET status=?,updated_at=? WHERE id=?", req.Status, nowSQL(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_status_failed")
		return
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		writeError(w, http.StatusNotFound, "group_not_found")
		return
	}
	a.audit(0, u.ID, "set_group_status", "study_groups", groupID, nil, map[string]any{"status": req.Status}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": req.Status})
}

func (a *app) handleSuperMergeUsers(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		PrimaryUserID   uint64 `json:"primary_user_id"`
		DuplicateUserID uint64 `json:"duplicate_user_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.PrimaryUserID == 0 || req.DuplicateUserID == 0 || req.PrimaryUserID == req.DuplicateUserID || req.DuplicateUserID == u.ID {
		writeError(w, http.StatusBadRequest, "user_merge_invalid")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_merge_failed")
		return
	}
	defer tx.Rollback()
	var primaryStatus, duplicateStatus int
	var primarySuper, duplicateSuper bool
	if err := tx.QueryRow("SELECT status,is_super_admin FROM users WHERE id=? FOR UPDATE", req.PrimaryUserID).Scan(&primaryStatus, &primarySuper); err != nil || primaryStatus != 1 {
		writeError(w, http.StatusBadRequest, "primary_user_unavailable")
		return
	}
	if err := tx.QueryRow("SELECT status,is_super_admin FROM users WHERE id=? FOR UPDATE", req.DuplicateUserID).Scan(&duplicateStatus, &duplicateSuper); err != nil || duplicateStatus != 1 || primarySuper || duplicateSuper {
		writeError(w, http.StatusBadRequest, "duplicate_user_unavailable")
		return
	}
	var conflicts int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM checkin_records d JOIN checkin_records p
		ON p.user_id=? AND p.group_id=d.group_id AND p.logical_date=d.logical_date AND p.deleted_at IS NULL
		AND ((p.task_type=d.task_type AND p.part=d.part) OR (p.task_id IS NOT NULL AND p.task_id=d.task_id))
		WHERE d.user_id=? AND d.deleted_at IS NULL`, req.PrimaryUserID, req.DuplicateUserID).Scan(&conflicts); err != nil || conflicts > 0 {
		writeError(w, http.StatusConflict, "user_merge_checkin_conflict")
		return
	}
	rows, err := tx.Query("SELECT id,group_id FROM group_members WHERE user_id=? FOR UPDATE", req.DuplicateUserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_merge_failed")
		return
	}
	type membership struct{ ID, GroupID uint64 }
	memberships := []membership{}
	for rows.Next() {
		var item membership
		if rows.Scan(&item.ID, &item.GroupID) == nil {
			memberships = append(memberships, item)
		}
	}
	rows.Close()
	for _, item := range memberships {
		var exists int
		_ = tx.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id=? AND user_id=?", item.GroupID, req.PrimaryUserID).Scan(&exists)
		if exists > 0 {
			if _, err := tx.Exec("DELETE FROM group_members WHERE id=?", item.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "user_merge_failed")
				return
			}
		} else if _, err := tx.Exec("UPDATE group_members SET user_id=?,updated_at=? WHERE id=?", req.PrimaryUserID, nowSQL(), item.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "user_merge_failed")
			return
		}
	}
	now := nowSQL()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT IGNORE INTO user_group_roles(group_id,user_id,role,created_at) SELECT group_id,?,role,created_at FROM user_group_roles WHERE user_id=?", []any{req.PrimaryUserID, req.DuplicateUserID}},
		{"DELETE FROM user_group_roles WHERE user_id=?", []any{req.DuplicateUserID}},
		{"UPDATE checkin_records SET user_id=?,created_by=IF(created_by=?, ?, created_by),updated_at=? WHERE user_id=?", []any{req.PrimaryUserID, req.DuplicateUserID, req.PrimaryUserID, now, req.DuplicateUserID}},
		{"UPDATE reflections SET user_id=?,updated_at=? WHERE user_id=?", []any{req.PrimaryUserID, now, req.DuplicateUserID}},
		{"UPDATE roster_entries SET claimed_by_user_id=?,updated_at=? WHERE claimed_by_user_id=?", []any{req.PrimaryUserID, now, req.DuplicateUserID}},
		{"UPDATE registration_requests SET applicant_user_id=?,updated_at=? WHERE applicant_user_id=?", []any{req.PrimaryUserID, now, req.DuplicateUserID}},
		{"UPDATE users SET status=0,auth_version=auth_version+1,default_group_id=NULL,updated_at=? WHERE id=?", []any{now, req.DuplicateUserID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.query, statement.args...); err != nil {
			writeError(w, http.StatusInternalServerError, "user_merge_failed")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "user_merge_failed")
		return
	}
	a.audit(0, u.ID, "merge_users", "users", req.PrimaryUserID, map[string]any{"duplicate_user_id": req.DuplicateUserID}, map[string]any{"duplicate_status": 0}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "primary_user_id": req.PrimaryUserID, "disabled_duplicate_user_id": req.DuplicateUserID})
}
