package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/draw"
)

const (
	maxAvatarFileBytes              = int64(6 << 20)
	maxAvatarMultipartOverheadBytes = int64(1 << 20)
	maxJPEGMetadataScanBytes        = int64(1 << 20)
)

type rosterRow struct {
	GroupCode string `json:"group_code"`
	GroupName string `json:"group_name"`
	Name      string `json:"name"`
	Identity  string `json:"identity_key"`
	IsLeader  bool   `json:"is_leader"`
	IsMinor   bool   `json:"is_minor"`
}

var rosterSheetCodes = map[string]string{
	"真理_DT+ZWTJ": "truth-dt-zwtj",
	"圣经_DT":      "bible-dt",
	"生命_ZWTJ":    "life-zwtj",
	"圣经_ZWWX":    "bible-zwwx",
	"圣经_KD":      "bible-kd",
	"圣经_AGP":     "bible-agp",
	"圣经_GD":      "bible-gd",
	"CZ_道路":      "cz-way",
}

func canonicalRosterName(value string) (string, bool) {
	v := strings.TrimSpace(value)
	minor := strings.Contains(v, "辅修")
	v = strings.ReplaceAll(v, "（辅修）", "")
	v = strings.ReplaceAll(v, "(辅修)", "")
	v = strings.TrimSpace(v)
	return v, minor
}

func identityKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), ""))
}

func usernameFromName(name string) string {
	overrides := map[string]string{
		"张迦勒": "zhangjiale", "陈思佳": "chensijia", "廖美倩": "liaomeiqian",
		"苏相宜": "suxiangyi", "李群": "liqun", "戴许诺": "daixunuo",
		"许水英": "xushuiying", "贺丽华": "helihua", "朱灵": "zhuling",
		"李思思": "lisisi", "何金群": "hejinqun", "胡方舟": "hufangzhou",
		"戴维多尔": "daiweiduoer", "仇健棒": "qiujianbang", "彭朋": "pengpeng",
	}
	if value := overrides[name]; value != "" {
		return value
	}
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	args.Heteronym = false
	var b strings.Builder
	for _, part := range pinyin.Pinyin(name, args) {
		if len(part) > 0 {
			b.WriteString(part[0])
		}
	}
	clean := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(b.String()), "")
	if clean == "" {
		clean = "member"
	}
	return clean
}

func (a *app) uniqueUsername(tx *sql.Tx, name string) (string, error) {
	base := usernameFromName(name)
	for i := 0; i < 10000; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s%d", base, i+1)
		}
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username)=?", candidate).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", errors.New("username_exhausted")
}

func parseRoster(path string) ([]rosterRow, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []rosterRow
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		nameColumn := rosterColumn(rows[0], "姓名", "成员姓名", "name")
		groupNameColumn := rosterColumn(rows[0], "小组名称", "小组", "组别", "group_name")
		groupCodeColumn := rosterColumn(rows[0], "小组编码", "组编码", "group_code")
		leaderColumn := rosterColumn(rows[0], "是否组长", "组长", "is_leader")
		minorColumn := rosterColumn(rows[0], "是否辅修", "辅修", "is_minor")
		_, legacySheet := rosterSheetCodes[strings.TrimSpace(sheet)]
		genericSheet := nameColumn >= 0 && (!legacySheet || groupNameColumn >= 0 || groupCodeColumn >= 0 || leaderColumn >= 0 || minorColumn >= 0)
		if genericSheet {
			for _, row := range rows[1:] {
				name, minor := canonicalRosterName(rosterCell(row, nameColumn))
				if name == "" {
					continue
				}
				groupName := strings.TrimSpace(rosterCell(row, groupNameColumn))
				if groupName == "" {
					groupName = strings.TrimSpace(sheet)
				}
				code := strings.ToLower(strings.TrimSpace(rosterCell(row, groupCodeColumn)))
				if code == "" {
					code = rosterSheetCodes[groupName]
				}
				if code == "" {
					code = stableGroupCode(groupName)
				}
				leader := rosterTruthy(rosterCell(row, leaderColumn))
				minor = minor || rosterTruthy(rosterCell(row, minorColumn))
				out = append(out, rosterRow{GroupCode: code, GroupName: groupName, Name: name, Identity: identityKey(name), IsLeader: leader, IsMinor: minor})
			}
			continue
		}
		code, ok := rosterSheetCodes[strings.TrimSpace(sheet)]
		if !ok {
			continue
		}
		for index, row := range rows {
			if index == 0 || len(row) < 2 || strings.TrimSpace(row[1]) == "" {
				continue
			}
			name, minor := canonicalRosterName(row[1])
			styleID, _ := f.GetCellStyle(sheet, fmt.Sprintf("B%d", index+1))
			style, _ := f.GetStyle(styleID)
			color := ""
			if style != nil && style.Font != nil {
				color = strings.ToUpper(strings.TrimPrefix(style.Font.Color, "#"))
			}
			leader := color == "FF0000" || color == "FFFF0000"
			out = append(out, rosterRow{GroupCode: code, GroupName: sheet, Name: name, Identity: identityKey(name), IsLeader: leader, IsMinor: minor})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no_supported_roster_sheets")
	}
	return out, nil
}

func rosterColumn(header []string, aliases ...string) int {
	for index, value := range header {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
		for _, alias := range aliases {
			if normalized == strings.ToLower(strings.ReplaceAll(alias, " ", "")) {
				return index
			}
		}
	}
	return -1
}

func rosterCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func rosterTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "是", "组长", "辅修":
		return true
	default:
		return false
	}
}

func stableGroupCode(name string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(name)))
	return "group-" + hex.EncodeToString(sum[:4])
}

func (a *app) bootstrapRoster() error {
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM roster_entries").Scan(&count); err != nil || count > 0 {
		return err
	}
	if _, err := os.Stat(a.rosterPath); err != nil {
		return err
	}
	rows, err := parseRoster(a.rosterPath)
	if err != nil {
		return err
	}
	data, _ := os.ReadFile(a.rosterPath)
	sum := sha256.Sum256(data)
	return a.syncRoster(rows, filepath.Base(a.rosterPath), hex.EncodeToString(sum[:]), 0)
}

func (a *app) syncRoster(rows []rosterRow, filename, checksum string, actor uint64) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowSQL()
	var actorValue any
	if actor > 0 {
		actorValue = actor
	}
	res, err := tx.Exec("INSERT INTO roster_imports(filename,checksum_sha256,imported_by,row_count,created_at) VALUES(?,?,?,?,?)", filename, checksum, actorValue, len(rows), now)
	if err != nil {
		return err
	}
	importID, _ := res.LastInsertId()
	seen := map[string]bool{}
	for _, row := range rows {
		_, err = tx.Exec(`INSERT INTO study_groups(code,name,description,status,created_by,created_at,updated_at)
			VALUES(?,?,?,1,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),status=1,updated_at=VALUES(updated_at)`, row.GroupCode, row.GroupName, "2026年度门训生命季", actorValue, now, now)
		if err != nil {
			return err
		}
		var groupID uint64
		if err = tx.QueryRow("SELECT id FROM study_groups WHERE code=?", row.GroupCode).Scan(&groupID); err != nil {
			return err
		}
		key := fmt.Sprintf("%d:%s", groupID, row.Identity)
		seen[key] = true
		_, err = tx.Exec(`INSERT INTO roster_entries(import_id,group_id,canonical_name,identity_key,is_leader,is_minor,status,created_at,updated_at)
			VALUES(?,?,?,?,?,?,1,?,?) ON DUPLICATE KEY UPDATE import_id=VALUES(import_id),canonical_name=VALUES(canonical_name),is_leader=VALUES(is_leader),is_minor=VALUES(is_minor),status=1,updated_at=VALUES(updated_at)`, importID, groupID, row.Name, row.Identity, row.IsLeader, row.IsMinor, now, now)
		if err != nil {
			return err
		}
	}
	current, err := tx.Query("SELECT id,group_id,identity_key,claimed_by_user_id,is_leader FROM roster_entries WHERE status=1")
	if err != nil {
		return err
	}
	type stale struct {
		id, gid uint64
		ident   string
		claimed sql.NullInt64
		leader  bool
	}
	var staleRows []stale
	for current.Next() {
		var s stale
		_ = current.Scan(&s.id, &s.gid, &s.ident, &s.claimed, &s.leader)
		if !seen[fmt.Sprintf("%d:%s", s.gid, s.ident)] {
			staleRows = append(staleRows, s)
		}
	}
	current.Close()
	for _, s := range staleRows {
		if _, err = tx.Exec("UPDATE roster_entries SET status=0,updated_at=? WHERE id=?", now, s.id); err != nil {
			return err
		}
		if s.claimed.Valid {
			_, _ = tx.Exec("UPDATE group_members SET status=0,updated_at=? WHERE group_id=? AND user_id=?", now, s.gid, s.claimed.Int64)
			if s.leader {
				_, _ = tx.Exec("DELETE FROM user_group_roles WHERE group_id=? AND user_id=? AND role=?", s.gid, s.claimed.Int64, roleGroupLeader)
			}
		}
	}
	// Apply leader changes immediately to already claimed accounts.
	_, _ = tx.Exec(`INSERT IGNORE INTO user_group_roles(group_id,user_id,role,created_at)
		SELECT group_id,claimed_by_user_id,?,? FROM roster_entries WHERE status=1 AND is_leader=1 AND claimed_by_user_id IS NOT NULL`, roleGroupLeader, now)
	_, _ = tx.Exec(`DELETE r FROM user_group_roles r JOIN roster_entries e ON e.group_id=r.group_id AND e.claimed_by_user_id=r.user_id
		WHERE r.role=? AND e.status=1 AND e.is_leader=0`, roleGroupLeader)
	return tx.Commit()
}

func (a *app) handleRegistrationGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT DISTINCT g.id,g.code,g.name FROM roster_entries e JOIN study_groups g ON g.id=e.group_id WHERE e.status=1 AND g.status=1 ORDER BY g.id`)
	if err != nil {
		writeError(w, 500, "groups_failed")
		return
	}
	defer rows.Close()
	groups := []group{}
	for rows.Next() {
		var g group
		_ = rows.Scan(&g.ID, &g.Code, &g.Name)
		groups = append(groups, g)
	}
	writeJSON(w, 200, map[string]any{"groups": groups})
}

func (a *app) handleRegistrationPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		GroupID uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	canonical, _ := canonicalRosterName(req.Name)
	var rosterID uint64
	var claimed sql.NullInt64
	err := a.db.QueryRow("SELECT id,claimed_by_user_id FROM roster_entries WHERE group_id=? AND identity_key=? AND status=1", req.GroupID, identityKey(canonical)).Scan(&rosterID, &claimed)
	if err != nil || claimed.Valid {
		writeError(w, 404, "roster_not_found")
		return
	}
	var pending int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM registration_requests WHERE roster_entry_id=? AND status='pending'", rosterID).Scan(&pending)
	if pending > 0 {
		writeError(w, http.StatusConflict, "registration_pending")
		return
	}
	writeJSON(w, 200, map[string]any{"matched": true})
}

func validEmail(v string) bool {
	i := strings.LastIndex(v, "@")
	return i > 0 && i < len(v)-3 && strings.Contains(v[i+1:], ".")
}

var registrationUsernamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

func validRegistrationPassword(value string) bool {
	if len(value) < 8 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, char := range value {
		hasLetter = hasLetter || unicode.IsLetter(char)
		hasDigit = hasDigit || unicode.IsDigit(char)
	}
	return hasLetter && hasDigit
}

func (a *app) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		GroupID  uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !registrationUsernamePattern.MatchString(username) {
		writeError(w, http.StatusBadRequest, "invalid_username")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email != "" && !validEmail(email) {
		writeError(w, 400, "invalid_email")
		return
	}
	if !validRegistrationPassword(req.Password) {
		writeError(w, 400, "weak_password")
		return
	}
	canonical, _ := canonicalRosterName(req.Name)
	var rosterID uint64
	var claimed sql.NullInt64
	err := a.db.QueryRow("SELECT id,claimed_by_user_id FROM roster_entries WHERE group_id=? AND identity_key=? AND status=1", req.GroupID, identityKey(canonical)).Scan(&rosterID, &claimed)
	if err != nil {
		writeError(w, 400, "roster_not_found")
		return
	}
	if claimed.Valid {
		writeError(w, 409, "roster_already_claimed")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, 500, "register_failed")
		return
	}
	defer tx.Rollback()
	var n int
	_ = tx.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username)=?", username).Scan(&n)
	if n > 0 {
		writeError(w, http.StatusConflict, "username_exists")
		return
	}
	if email != "" {
		_ = tx.QueryRow("SELECT COUNT(*) FROM users WHERE email_normalized=?", email).Scan(&n)
	}
	if email != "" && n > 0 {
		writeError(w, 409, "email_exists")
		return
	}
	_ = tx.QueryRow("SELECT COUNT(*) FROM registration_requests WHERE roster_entry_id=? AND status='pending'", rosterID).Scan(&n)
	if n > 0 {
		writeError(w, http.StatusConflict, "registration_pending")
		return
	}
	_ = tx.QueryRow("SELECT COUNT(*) FROM registration_requests WHERE LOWER(username)=? AND status='pending'", username).Scan(&n)
	if n > 0 {
		writeError(w, http.StatusConflict, "username_exists")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, 500, "register_failed")
		return
	}
	now := nowSQL()
	var emailValue any
	if email != "" {
		emailValue = email
	}
	res, err := tx.Exec(`INSERT INTO registration_requests(roster_entry_id,group_id,canonical_name,username,email_normalized,password_hash,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,'pending',?,?)`, rosterID, req.GroupID, canonical, username, emailValue, hash, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "register_failed")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "register_failed")
		return
	}
	requestID, _ := res.LastInsertId()
	writeJSON(w, http.StatusAccepted, map[string]any{"request_id": requestID, "status": "pending", "username": username})
}

func (a *app) handleRegistrationRequests(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	if u.CurrentGroupID == 0 {
		writeError(w, http.StatusBadRequest, "group_required")
		return
	}
	rows, err := a.db.Query(`SELECT rr.id,rr.canonical_name,rr.username,rr.email_normalized,rr.status,rr.rejection_reason,rr.created_at,re.canonical_name
		FROM registration_requests rr JOIN roster_entries re ON re.id=rr.roster_entry_id
		WHERE rr.group_id=? ORDER BY FIELD(rr.status,'pending','rejected','approved'),rr.created_at`, u.CurrentGroupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_requests_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uint64
		var name, username, status, reason, rosterName string
		var email sql.NullString
		var created time.Time
		if rows.Scan(&id, &name, &username, &email, &status, &reason, &created, &rosterName) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "roster_name": rosterName, "username": username, "email": email.String, "status": status, "rejection_reason": reason, "created_at": created.Format(time.RFC3339)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": items})
}

func (a *app) handleApproveRegistration(w http.ResponseWriter, r *http.Request) {
	a.reviewRegistration(w, r, true)
}

func (a *app) handleRejectRegistration(w http.ResponseWriter, r *http.Request) {
	a.reviewRegistration(w, r, false)
}

func (a *app) reviewRegistration(w http.ResponseWriter, r *http.Request, approve bool) {
	u := mustUser(r)
	requestID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if requestID == 0 || u.CurrentGroupID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_registration_request")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !approve && !readJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if !approve && reason == "" {
		writeError(w, http.StatusBadRequest, "rejection_reason_required")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_review_failed")
		return
	}
	defer tx.Rollback()
	var rosterID, groupID uint64
	var name, username, passwordHash, status string
	var email sql.NullString
	err = tx.QueryRow(`SELECT roster_entry_id,group_id,canonical_name,username,email_normalized,password_hash,status
		FROM registration_requests WHERE id=? FOR UPDATE`, requestID).Scan(&rosterID, &groupID, &name, &username, &email, &passwordHash, &status)
	if err != nil || status != "pending" || (!u.IsSuperAdmin && groupID != u.CurrentGroupID) || groupID != u.CurrentGroupID {
		writeError(w, http.StatusConflict, "registration_request_unavailable")
		return
	}
	now := nowSQL()
	if !approve {
		_, err = tx.Exec("UPDATE registration_requests SET status='rejected',password_hash='',rejection_reason=?,reviewed_by=?,reviewed_at=?,updated_at=? WHERE id=? AND status='pending'", reason, u.ID, now, now, requestID)
		if err != nil || tx.Commit() != nil {
			writeError(w, http.StatusInternalServerError, "registration_review_failed")
			return
		}
		a.audit(groupID, u.ID, "reject_registration", "registration_request", requestID, map[string]any{"status": "pending"}, map[string]any{"status": "rejected", "reason": reason}, r)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "rejected"})
		return
	}
	var claimed sql.NullInt64
	var leader bool
	err = tx.QueryRow("SELECT claimed_by_user_id,is_leader FROM roster_entries WHERE id=? AND group_id=? AND status=1 FOR UPDATE", rosterID, groupID).Scan(&claimed, &leader)
	if err != nil || claimed.Valid {
		writeError(w, http.StatusConflict, "roster_already_claimed")
		return
	}
	var exists int
	_ = tx.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username)=?", username).Scan(&exists)
	if exists > 0 {
		writeError(w, http.StatusConflict, "username_exists")
		return
	}
	if email.Valid {
		_ = tx.QueryRow("SELECT COUNT(*) FROM users WHERE email_normalized=?", email.String).Scan(&exists)
		if exists > 0 {
			writeError(w, http.StatusConflict, "email_exists")
			return
		}
	}
	userEmail := ""
	var normalizedEmail any
	if email.Valid {
		userEmail = email.String
		normalizedEmail = email.String
	}
	res, err := tx.Exec(`INSERT INTO users(username,display_name,name_pinyin,password_hash,email,email_normalized,default_group_id,status,created_by,created_at,updated_at,profile_updated_at)
		VALUES(?,?,?,?,?,?,?,1,?,?,?,?)`, username, name, username, passwordHash, userEmail, normalizedEmail, groupID, u.ID, now, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "account_exists")
		return
	}
	userID64, _ := res.LastInsertId()
	userID := uint64(userID64)
	res, err = tx.Exec("UPDATE roster_entries SET claimed_by_user_id=?,updated_at=? WHERE id=? AND claimed_by_user_id IS NULL", userID, now, rosterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_review_failed")
		return
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		writeError(w, http.StatusConflict, "roster_already_claimed")
		return
	}
	_, err = tx.Exec(`INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_by,created_at,updated_at)
		VALUES(?,?,?,1,?,?,?,?)`, groupID, userID, name, now, u.ID, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_review_failed")
		return
	}
	if leader {
		_, err = tx.Exec("INSERT IGNORE INTO user_group_roles(group_id,user_id,role,created_at) VALUES(?,?,?,?)", groupID, userID, roleGroupLeader, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "registration_review_failed")
			return
		}
	}
	_, err = tx.Exec("UPDATE registration_requests SET status='approved',password_hash='',reviewed_by=?,reviewed_at=?,updated_at=? WHERE id=? AND status='pending'", u.ID, now, now, requestID)
	if err != nil || tx.Commit() != nil {
		writeError(w, http.StatusInternalServerError, "registration_review_failed")
		return
	}
	a.audit(groupID, u.ID, "approve_registration", "registration_request", requestID, map[string]any{"status": "pending"}, map[string]any{"status": "approved", "user_id": userID}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "approved", "user_id": userID})
}

func (a *app) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`).MatchString(username) {
		writeError(w, 400, "invalid_username")
		return
	}
	if !validEmail(email) {
		writeError(w, 400, "invalid_email")
		return
	}
	_, err := a.db.Exec("UPDATE users SET username=?,name_pinyin=?,email=?,email_normalized=?,profile_updated_at=?,updated_at=? WHERE id=?", username, username, email, email, nowSQL(), nowSQL(), u.ID)
	if err != nil {
		writeError(w, 409, "profile_conflict")
		return
	}
	updated, _ := a.loadCurrentUser(u.ID, u.CurrentGroupID)
	writeJSON(w, 200, map[string]any{"user": updated})
}

func (a *app) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarFileBytes+maxAvatarMultipartOverheadBytes)
	if err := r.ParseMultipartForm(maxAvatarFileBytes); err != nil {
		writeError(w, 400, "avatar_too_large")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, 400, "avatar_required")
		return
	}
	defer file.Close()
	if !validAvatarFileSize(header.Size) {
		writeError(w, 400, "avatar_too_large")
		return
	}
	config, format, err := image.DecodeConfig(file)
	if err != nil || validateAvatarDimensions(config.Width, config.Height) != nil {
		writeError(w, 400, "invalid_avatar")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		writeError(w, 400, "invalid_avatar")
		return
	}
	orientation := 1
	if format == "jpeg" {
		orientation = readJPEGEXIFOrientation(file)
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			writeError(w, 400, "invalid_avatar")
			return
		}
	}
	img, _, err := image.Decode(file)
	if err != nil {
		writeError(w, 400, "invalid_avatar")
		return
	}
	img = applyEXIFOrientation(img, orientation)
	b := img.Bounds()
	if err := validateAvatarDimensions(b.Dx(), b.Dy()); err != nil {
		writeError(w, 400, "invalid_avatar")
		return
	}
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	src := image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, b.Min.X+(b.Dx()+side)/2, b.Min.Y+(b.Dy()+side)/2)
	dst := image.NewRGBA(image.Rect(0, 0, 512, 512))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, src, draw.Over, nil)
	dir := filepath.Join(a.assetsRoot, "avatars")
	if err = os.MkdirAll(dir, 0755); err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	name := fmt.Sprintf("%d-%d.jpg", u.ID, time.Now().UnixNano())
	finalPath := filepath.Join(dir, name)
	out, err := os.CreateTemp(dir, ".avatar-*.jpg")
	if err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	tempPath := out.Name()
	defer os.Remove(tempPath)
	encodeErr := jpeg.Encode(out, dst, &jpeg.Options{Quality: 86})
	closeErr := out.Close()
	if encodeErr != nil || closeErr != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	keepFinal := false
	defer func() {
		if !keepFinal {
			_ = os.Remove(finalPath)
		}
	}()
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	defer tx.Rollback()
	var oldPath string
	if err = tx.QueryRow("SELECT avatar_path FROM users WHERE id=? FOR UPDATE", u.ID).Scan(&oldPath); err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	if _, err = tx.Exec("UPDATE users SET avatar_path=?,profile_updated_at=?,updated_at=? WHERE id=?", name, nowSQL(), nowSQL(), u.ID); err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "avatar_failed")
		return
	}
	keepFinal = true
	if oldName := avatarFilename(oldPath); oldName != "" && oldName != name {
		_ = os.Remove(filepath.Join(dir, oldName))
	}
	writeJSON(w, 200, map[string]any{"avatar_url": "/api/avatars/" + name})
}

func validateAvatarDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > maxAvatarSide || height > maxAvatarSide {
		return errors.New("invalid_avatar_dimensions")
	}
	if int64(width)*int64(height) > maxAvatarPixels {
		return errors.New("avatar_too_many_pixels")
	}
	return nil
}

func validAvatarFileSize(size int64) bool {
	return size >= 0 && size <= maxAvatarFileBytes
}

func readJPEGEXIFOrientation(r io.Reader) int {
	var signature [2]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil || signature != [2]byte{0xff, 0xd8} {
		return 1
	}
	scanned := int64(len(signature))
	var one [1]byte
	for scanned < maxJPEGMetadataScanBytes {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return 1
		}
		scanned++
		if one[0] != 0xff {
			continue
		}
		for {
			if _, err := io.ReadFull(r, one[:]); err != nil {
				return 1
			}
			scanned++
			if one[0] != 0xff {
				break
			}
		}
		marker := one[0]
		if marker == 0x00 || marker == 0xd8 || marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if marker == 0xd9 || marker == 0xda {
			return 1
		}
		var lengthBytes [2]byte
		if _, err := io.ReadFull(r, lengthBytes[:]); err != nil {
			return 1
		}
		scanned += int64(len(lengthBytes))
		segmentLength := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if segmentLength < 2 {
			return 1
		}
		payloadLength := segmentLength - 2
		if scanned+int64(payloadLength) > maxJPEGMetadataScanBytes {
			return 1
		}
		if marker == 0xe1 {
			payload := make([]byte, payloadLength)
			if _, err := io.ReadFull(r, payload); err != nil {
				return 1
			}
			scanned += int64(payloadLength)
			if orientation, ok := parseEXIFOrientation(payload); ok {
				return orientation
			}
			continue
		}
		if _, err := io.CopyN(io.Discard, r, int64(payloadLength)); err != nil {
			return 1
		}
		scanned += int64(payloadLength)
	}
	return 1
}

func parseEXIFOrientation(payload []byte) (int, bool) {
	if len(payload) < 14 || string(payload[:6]) != "Exif\x00\x00" {
		return 1, false
	}
	tiff := payload[6:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1, false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1, false
	}
	ifdOffset := order.Uint32(tiff[4:8])
	if ifdOffset > uint32(len(tiff)-2) {
		return 1, false
	}
	entryCountOffset := int(ifdOffset)
	entryCount := int(order.Uint16(tiff[entryCountOffset : entryCountOffset+2]))
	entriesOffset := entryCountOffset + 2
	if entryCount > (len(tiff)-entriesOffset)/12 {
		return 1, false
	}
	for index := 0; index < entryCount; index++ {
		entry := tiff[entriesOffset+index*12 : entriesOffset+(index+1)*12]
		if order.Uint16(entry[0:2]) != 0x0112 {
			continue
		}
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			return 1, false
		}
		orientation := int(order.Uint16(entry[8:10]))
		return orientation, orientation >= 1 && orientation <= 8
	}
	return 1, false
}

type exifOrientedImage struct {
	source      image.Image
	orientation int
	bounds      image.Rectangle
}

func applyEXIFOrientation(source image.Image, orientation int) image.Image {
	if source == nil || orientation < 2 || orientation > 8 {
		return source
	}
	sourceBounds := source.Bounds()
	width, height := sourceBounds.Dx(), sourceBounds.Dy()
	orientedBounds := image.Rect(0, 0, width, height)
	if orientation >= 5 {
		orientedBounds = image.Rect(0, 0, height, width)
	}
	return &exifOrientedImage{source: source, orientation: orientation, bounds: orientedBounds}
}

func (img *exifOrientedImage) ColorModel() color.Model {
	return img.source.ColorModel()
}

func (img *exifOrientedImage) Bounds() image.Rectangle {
	return img.bounds
}

func (img *exifOrientedImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(img.bounds) {
		return color.RGBA{}
	}
	sourceBounds := img.source.Bounds()
	width, height := sourceBounds.Dx(), sourceBounds.Dy()
	var sourceX, sourceY int
	switch img.orientation {
	case 2:
		sourceX, sourceY = width-1-x, y
	case 3:
		sourceX, sourceY = width-1-x, height-1-y
	case 4:
		sourceX, sourceY = x, height-1-y
	case 5:
		sourceX, sourceY = y, x
	case 6:
		sourceX, sourceY = y, height-1-x
	case 7:
		sourceX, sourceY = width-1-y, height-1-x
	case 8:
		sourceX, sourceY = width-1-y, x
	default:
		sourceX, sourceY = x, y
	}
	return img.source.At(sourceBounds.Min.X+sourceX, sourceBounds.Min.Y+sourceY)
}

func avatarFilename(value string) string {
	value = strings.TrimSpace(value)
	base := filepath.Base(value)
	if value != base || base == "" || base == "." || base == ".." || strings.ContainsAny(base, "/\\\x00") {
		return ""
	}
	return base
}

func isVersionedAvatarFilename(name string) bool {
	stem, ok := strings.CutSuffix(name, ".jpg")
	if !ok {
		return false
	}
	userID, timestamp, ok := strings.Cut(stem, "-")
	if !ok || strings.Contains(timestamp, "-") || userID == "" || timestamp == "" {
		return false
	}
	for _, part := range []string{userID, timestamp} {
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func (a *app) handleAvatar(w http.ResponseWriter, r *http.Request) {
	name := avatarFilename(r.PathValue("name"))
	if name == "" {
		http.NotFound(w, r)
		return
	}
	if isVersionedAvatarFilename(name) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFile(w, r, filepath.Join(a.assetsRoot, "avatars", name))
}

func saveRosterUpload(w http.ResponseWriter, r *http.Request) (string, string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		return "", "", "", err
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()
	tmp, err := os.CreateTemp("", "agp-roster-*.xlsx")
	if err != nil {
		return "", "", "", err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, hash), f)
	tmp.Close()
	if err != nil {
		os.Remove(tmp.Name())
		return "", "", "", err
	}
	return tmp.Name(), h.Filename, hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *app) handleSuperRoster(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT e.id,e.canonical_name,e.is_leader,e.is_minor,e.status,e.claimed_by_user_id,g.id,g.name,u.username,u.email FROM roster_entries e JOIN study_groups g ON g.id=e.group_id LEFT JOIN users u ON u.id=e.claimed_by_user_id ORDER BY g.id,e.id`)
	if err != nil {
		writeError(w, 500, "roster_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, gid uint64
		var name, gname string
		var leader, minor bool
		var status int
		var claimed sql.NullInt64
		var username, email sql.NullString
		_ = rows.Scan(&id, &name, &leader, &minor, &status, &claimed, &gid, &gname, &username, &email)
		items = append(items, map[string]any{"id": id, "name": name, "group_id": gid, "group_name": gname, "is_leader": leader, "is_minor": minor, "status": status, "claimed_by_user_id": nullableUint64(claimed), "username": username.String, "email": email.String})
	}
	writeJSON(w, 200, map[string]any{"entries": items})
}

func (a *app) handleSuperRosterPreview(w http.ResponseWriter, r *http.Request) {
	path, name, checksum, err := saveRosterUpload(w, r)
	if err != nil {
		writeError(w, 400, "invalid_roster_file")
		return
	}
	defer os.Remove(path)
	rows, err := parseRoster(path)
	if err != nil {
		writeError(w, 400, "invalid_roster_file")
		return
	}
	leaders := 0
	for _, row := range rows {
		if row.IsLeader {
			leaders++
		}
	}
	writeJSON(w, 200, map[string]any{"filename": name, "checksum": checksum, "row_count": len(rows), "leader_count": leaders, "entries": rows})
}

func (a *app) handleSuperRosterImport(w http.ResponseWriter, r *http.Request) {
	path, name, checksum, err := saveRosterUpload(w, r)
	if err != nil {
		writeError(w, 400, "invalid_roster_file")
		return
	}
	defer os.Remove(path)
	rows, err := parseRoster(path)
	if err != nil {
		writeError(w, 400, "invalid_roster_file")
		return
	}
	if err = a.syncRoster(rows, name, checksum, mustUser(r).ID); err != nil {
		writeError(w, 500, "roster_import_failed")
		return
	}
	a.audit(0, mustUser(r).ID, "import_roster", "roster_entries", 0, nil, map[string]any{"filename": name, "rows": len(rows)}, r)
	writeJSON(w, 200, map[string]any{"imported": len(rows)})
}

func (a *app) handleSuperRosterEntry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       uint64 `json:"id"`
		GroupID  uint64 `json:"group_id"`
		Name     string `json:"name"`
		IsLeader bool   `json:"is_leader"`
		IsMinor  bool   `json:"is_minor"`
		Status   int    `json:"status"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, _ := canonicalRosterName(req.Name)
	if name == "" || req.GroupID == 0 {
		writeError(w, 400, "invalid_roster_entry")
		return
	}
	now := nowSQL()
	if req.Status == 0 {
		req.Status = 1
	}
	if req.ID == 0 {
		_, err := a.db.Exec("INSERT INTO roster_entries(group_id,canonical_name,identity_key,is_leader,is_minor,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", req.GroupID, name, identityKey(name), req.IsLeader, req.IsMinor, req.Status, now, now)
		if err != nil {
			writeError(w, 409, "roster_conflict")
			return
		}
	} else {
		_, err := a.db.Exec("UPDATE roster_entries SET group_id=?,canonical_name=?,identity_key=?,is_leader=?,is_minor=?,status=?,updated_at=? WHERE id=?", req.GroupID, name, identityKey(name), req.IsLeader, req.IsMinor, req.Status, now, req.ID)
		if err != nil {
			writeError(w, 409, "roster_conflict")
			return
		}
	}
	var claimed sql.NullInt64
	_ = a.db.QueryRow("SELECT claimed_by_user_id FROM roster_entries WHERE group_id=? AND identity_key=?", req.GroupID, identityKey(name)).Scan(&claimed)
	if claimed.Valid {
		if req.IsLeader {
			_, _ = a.db.Exec("INSERT IGNORE INTO user_group_roles(group_id,user_id,role,created_at) VALUES(?,?,?,?)", req.GroupID, claimed.Int64, roleGroupLeader, now)
		} else {
			_, _ = a.db.Exec("DELETE FROM user_group_roles WHERE group_id=? AND user_id=? AND role=?", req.GroupID, claimed.Int64, roleGroupLeader)
		}
	}
	a.audit(req.GroupID, mustUser(r).ID, "save_roster_entry", "roster_entries", req.ID, nil, map[string]any{"name": name, "is_leader": req.IsLeader, "is_minor": req.IsMinor, "status": req.Status}, r)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func onlyLettersAndNumbers(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}
