<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import AppIcon from './AppIcon.vue';
import AppDatePicker from './AppDatePicker.vue';
import { useAppStateStore } from '../stores/appState';
import { confirmDialog, promptDialog } from '../ui/dialog';
import {
  addWeekBinding, api, applyBindingSelection, applyOutlineSelection, createBlankWeekDraft, createWeekDraft,
  deleteWeekDraft, downloadAdminExport, importLocalBackupJSON, importStudyWeeksExcel,
  librarySelectionValue, loadAdminData, previewLibraryItem, removeMember, removeWeekBinding,
  reloadApp, saveWeekDraft, selectWeekForEditing,
  saveLearningConfig, setAdminSection, setMemberAdmin, setResourceLibraryVisibility, switchGroup, toast, updateLearningValue, updateWeekBinding,
  updateWeekDraftField, uploadLibraryFile,
} from '../legacy-app';

const store = useAppStateStore();
const { user, adminSection, members, weeks, weekDraft, learningConfig, resourceLibrary, canEditLearning, adminLoading, groups, currentGroupID } = storeToRefs(store);

const uploadInput = ref(null);
const uploadCategory = ref('book');
const rosterInput = ref(null);
const rosterPreview = ref(null);
const rosterEntries = ref([]);
const registrationRequests = ref([]);
const rosterName = ref('');
const rosterGroupID = ref('');
const rosterLeader = ref(false);
const rosterMinor = ref(false);
const studyWeeksInput = ref(null);
const backupInput = ref(null);
const legacyConfigInput = ref(null);
const legacyRecordsInput = ref(null);
const legacyPreview = ref(null);
const activeLibraryKey = ref('');
const activeLibraryFolder = ref('');
const newFolderName = ref('');
const resourceSearch = ref('');
const rosterSearch = ref('');
const pendingAction = ref('');
const newGroupCode = ref('');
const newGroupName = ref('');
const newGroupDescription = ref('');
const taskPreviewOpen = ref(false);
const plannerStep = ref(1);
const adminNav = ref(null);
const checkinRows = ref([]);
const checkinFilters = ref({ from: localDateKey(new Date(Date.now() - 6 * 86400000)), to: localDateKey(), user_id: '' });
const checkinForm = ref({ user_id: '', task_type: 'daily_devotion', logical_date: localDateKey(), detail: '', note: '' });
const completionStats = ref([]);
const statisticsRange = ref({ from: localDateKey(new Date(Date.now() - 6 * 86400000)), to: localDateKey() });
const groupSettings = ref({ group: { name: '', description: '' }, options: { retro_days: 30, show_group_summary: true, show_member_status: false, show_reflections: false, allow_member_ranking: false, site_title: '', home_message: '' } });
const auditLogs = ref([]);
const superOverview = ref(null);
const superUsers = ref([]);
const platformGroups = ref([]);
const mergeUsers = ref({ primary_user_id: '', duplicate_user_id: '' });
let adminNavMobileQuery = null;

const libraryItems = computed(() => resourceLibrary.value.flatMap((section) => section.items || []));
const readingOptions = computed(() => libraryItems.value.filter((item) => ['markdown', 'pdf'].includes(item.type)));
const outlineOptions = computed(() => libraryItems.value.filter((item) => item.type === 'image'));
const activeLibrarySection = computed(() => resourceLibrary.value.find((section) => section.key === activeLibraryKey.value) || resourceLibrary.value[0] || null);
const visibleLibraryItems = computed(() => {
  let items = activeLibrarySection.value?.items || [];
  if (activeLibraryFolder.value) items = items.filter((item) => item.folder === activeLibraryFolder.value);
  const keyword = normalizedName(resourceSearch.value);
  if (!keyword) return items;
  return items.filter((item) => normalizedName(`${item.title || ''} ${item.original_name || ''} ${item.relative_path || ''} ${item.folder || ''}`).includes(keyword));
});
const currentWeek = computed(() => weeks.value.find((week) => week.publication_status === 'published' && weekStatus(week) === '进行中') || weeks.value.find((week) => week.publication_status === 'published'));
const completedProfiles = computed(() => members.value.filter((item) => item.username).length);
const pendingRegistrationCount = computed(() => registrationRequests.value.filter((item) => item.status === 'pending').length);
const activeGroupName = computed(() => groups.value.find((group) => Number(group.id) === Number(currentGroupID.value))?.name || '当前小组');
const isReadOnlyAdmin = computed(() => !canEditLearning.value);
const dailySettings = computed(() => learningConfig.value?.task_sections?.daily || {});
const dailyDevotion = computed(() => dailySettings.value.devotion || {});
const dailyScripture = computed(() => dailySettings.value.scripture || {});
const shareSettings = computed(() => learningConfig.value?.task_sections?.share || {});
const taskProgress = computed(() => {
  const draft = weekDraft.value || {};
  const items = [
    dailyDevotion.value.enabled === false || Boolean(dailyDevotion.value.title || dailyDevotion.value.path),
    dailyScripture.value.enabled === false || Boolean(dailyScripture.value.label || dailyScripture.value.book),
    draft.book_enabled === false || Boolean((draft.readings || []).some((item) => item.title || item.url || item.asset_id)),
    draft.video_enabled === false || Boolean((draft.videos || []).some((item) => item.title || item.url || item.asset_id)),
    draft.verse_enabled === false || Boolean(draft.verse_ref && draft.recite_text),
    draft.outline_enabled === false || Boolean(draft.outline?.title || draft.outline?.url || draft.outline?.asset_id),
    shareSettings.value.enabled === false || Boolean(shareSettings.value.label),
  ];
  return { done: items.filter(Boolean).length, total: items.length };
});
const publishErrors = computed(() => {
  const errors = [];
  const draft = weekDraft.value || {};
  const dateError = validateWeekDates();
  if (dateError) errors.push(dateError);
  if (!String(draft.title || '').trim()) errors.push('请填写本周主题');
  if (draft.book_enabled !== false && !(draft.readings || []).some((item) => item.title || item.url || item.asset_id)) errors.push('周读物已开启，请添加至少一项内容');
  if (draft.video_enabled !== false && !(draft.videos || []).some((item) => item.title || item.url || item.asset_id)) errors.push('本周视频已开启，请添加至少一项内容');
  if (draft.verse_enabled !== false && !String(draft.verse_ref || '').trim()) errors.push('背经已开启，请填写经文范围');
  if (draft.verse_enabled !== false && !String(draft.recite_text || '').trim()) errors.push('背经已开启，请填写经文原文');
  if (draft.outline_enabled !== false && !(draft.outline?.title || draft.outline?.url || draft.outline?.asset_id)) errors.push('提纲已开启，请选择或上传图片');
  return errors;
});
const canManageResources = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role))));
const resourceTargetWeek = computed(() => {
  if (!weekDraft.value) return null;
  if (!weekDraft.value.id) return weekDraft.value;
  return weeks.value.find((week) => Number(week.id) === Number(weekDraft.value.id)) || weekDraft.value;
});
const resourceTargetLabel = computed(() => {
  const target = resourceTargetWeek.value;
  if (!target) return '尚未选择周任务';
  const title = target.title || (target.id ? '未命名周任务' : '尚未保存的新周任务');
  return target.start && target.end ? `${title}（${target.start} — ${target.end}）` : title;
});
const filteredRosterEntries = computed(() => {
  const keyword = normalizedName(rosterSearch.value);
  if (!keyword) return rosterEntries.value;
  return rosterEntries.value.filter((entry) => normalizedName([
    entry.name,
    entry.group_name,
    entry.is_minor ? '辅修' : '主修',
    entry.is_leader ? '组长' : '',
    entry.claimed_by_user_id ? '已注册' : '待注册',
  ].join(' ')).includes(keyword));
});

watch(resourceLibrary, (sections) => {
  if (!sections?.some((section) => section.key === activeLibraryKey.value)) activeLibraryKey.value = sections?.[0]?.key || '';
}, { immediate: true });
watch(activeLibrarySection, (section) => {
  if (activeLibraryFolder.value && !(section?.folders || []).some((folder) => folder.path === activeLibraryFolder.value)) activeLibraryFolder.value = '';
});
watch(adminSection, () => centerActiveAdminSection());

const sections = computed(() => [
  ...(user.value?.is_super_admin ? [{ id: 'groups', label: `小组管理 (${groups.value.length})`, icon: 'users' }] : []),
  { id: 'overview', label: '管理概览', icon: 'chart' },
  { id: 'learning', label: '本周任务', icon: 'calendar' },
  { id: 'approvals', label: `注册审批${pendingRegistrationCount.value ? ` (${pendingRegistrationCount.value})` : ''}`, icon: 'check' },
  { id: 'members', label: '成员权限', icon: 'users' },
  { id: 'checkins', label: '打卡记录', icon: 'check' },
  { id: 'library', label: '学习资源', icon: 'library' },
  { id: 'statistics', label: '数据统计', icon: 'chart' },
  { id: 'settings', label: '小组设置', icon: 'settings' },
  { id: 'audit', label: '审计记录', icon: 'file' },
  ...(user.value?.is_super_admin ? [{ id: 'roster', label: '报名名单', icon: 'file' }] : []),
  ...(user.value?.is_super_admin ? [{ id: 'accounts', label: '全体账号', icon: 'users' }] : []),
  ...(user.value?.is_super_admin ? [{ id: 'operations', label: '系统运维', icon: 'database' }] : []),
  { id: 'data', label: '数据工具', icon: 'database' },
]);

function localDateKey(date = new Date()) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function centerActiveAdminSection(behavior = 'smooth') {
  nextTick(() => {
    const nav = adminNav.value;
    if (!nav || !window.matchMedia('(max-width: 900px)').matches) return;
    const active = nav.querySelector('[aria-current="page"]');
    if (!active) return;
    const navRect = nav.getBoundingClientRect();
    const activeRect = active.getBoundingClientRect();
    const offset = activeRect.left - navRect.left - (nav.clientWidth - activeRect.width) / 2;
    const left = Math.max(0, Math.min(nav.scrollWidth - nav.clientWidth, nav.scrollLeft + offset));
    if (Math.abs(left - nav.scrollLeft) > 1) nav.scrollTo({ left, behavior });
  });
}

function handleAdminNavViewportChange(event) {
  if (event.matches) centerActiveAdminSection('auto');
}

function weekStatus(week) {
  const today = localDateKey();
  if (!week) return '未设置';
  if (today < week.start) return '即将开始';
  if (today > week.end) return '已结束';
  return '进行中';
}

function roleLabel(member) {
  if (member.is_super_admin) return '超级管理员';
  if (member.roles?.includes('group_leader')) return '组长';
  if (member.roles?.includes('group_admin')) return '管理员';
  return '成员';
}

function optionText(item) { return item.title || item.original_name || '未命名资源'; }
function isPending(key) { return pendingAction.value === key; }
async function withPending(key, action) {
  if (pendingAction.value) return;
  pendingAction.value = key;
  try { return await action(); }
  finally { pendingAction.value = ''; }
}

function validateWeekDates() {
  const draft = weekDraft.value;
  if (!draft?.start || !draft?.end) return '请完整填写开始日期和结束日期';
  if (draft.start > draft.end) return '结束日期不能早于开始日期';
  const conflict = weeks.value.find((week) => Number(week.id) !== Number(draft.id || 0) && draft.start <= week.end && draft.end >= week.start);
  if (conflict) return `日期与“${conflict.title || '未命名周任务'}”（${conflict.start} — ${conflict.end}）重叠`;
  return '';
}

async function saveWeek(status) {
  if (!canEditLearning.value) return toast('当前账号只有任务只读权限');
  const dateError = validateWeekDates();
  if (dateError) return toast(dateError);
  if (status === 'published' && publishErrors.value.length) return toast(publishErrors.value[0]);
  if (status === 'published' && weekDraft.value?.publication_status === 'published') {
    const confirmed = await confirmDialog({ title: '确认更新已发布任务', message: '成员会立即看到本次修改，历史打卡记录不会改变。确认重新发布吗？', confirmLabel: '确认发布' });
    if (!confirmed) return;
  }
  await withPending(`save-week-${status}`, async () => {
    await saveLearningConfig();
    await saveWeekDraft(status);
    taskPreviewOpen.value = false;
    plannerStep.value = status === 'published' ? 3 : 2;
  });
}

function previewWeekDraft() {
  if (publishErrors.value.length) return toast(publishErrors.value[0]);
  taskPreviewOpen.value = true;
  plannerStep.value = 3;
}

function prepareNextWeek(blank = false) {
  if (blank) createBlankWeekDraft(); else createWeekDraft();
  taskPreviewOpen.value = false;
  plannerStep.value = 1;
}

function chooseWeek(weekID) {
  selectWeekForEditing(weekID);
  taskPreviewOpen.value = false;
  plannerStep.value = 1;
}

async function uploadTaskFile(kind, index, event) {
  const file = event.target.files?.[0];
  if (!file || !canEditLearning.value) return;
  const key = `task-upload-${kind}-${index}`;
  await withPending(key, async () => {
    const form = new FormData();
    form.append('category', kind === 'readings' ? 'book' : kind === 'videos' ? 'video' : 'outline');
    form.append('file', file);
    try {
      const result = await api('/admin/assets/upload', { method: 'POST', body: form, timeout: 15 * 60 * 1000 });
      const asset = result.asset;
      if (kind === 'outline') {
        updateWeekDraftField('outline', { title: asset.title || asset.original_name, url: '', type: 'image', asset_id: Number(asset.id) });
      } else {
        updateWeekBinding(kind, index, 'title', asset.title || asset.original_name || '学习资料');
        updateWeekBinding(kind, index, 'url', '');
        updateWeekBinding(kind, index, 'asset_id', Number(asset.id));
        updateWeekBinding(kind, index, 'type', kind === 'videos' ? 'video' : 'pdf');
      }
      await loadAdminData(true, true);
      toast('文件已上传并挂载到当前任务');
    } catch (error) { toast(`上传失败：${error.message}`); }
    finally { event.target.value = ''; }
  });
}

async function deleteWeekWithPending() {
  if (!canEditLearning.value) return;
  await withPending('delete-week', deleteWeekDraft);
}

async function changeMemberAdmin(member) {
  const key = `member-role-${member.member_id}`;
  await withPending(key, () => setMemberAdmin(member, !member.roles?.includes('group_admin')));
}

async function removeMemberWithPending(member) {
  const key = `remove-member-${member.member_id}`;
  await withPending(key, () => removeMember(member));
}

async function loadCheckins() {
  const params = new URLSearchParams({ from: checkinFilters.value.from, to: checkinFilters.value.to, page_size: '200' });
  if (checkinFilters.value.user_id) params.set('user_id', checkinFilters.value.user_id);
  const data = await api(`/checkins?${params}`);
  checkinRows.value = data.items || [];
}

async function createAdminCheckin() {
  const form = checkinForm.value;
  if (!form.user_id || !form.logical_date) return toast('请选择成员和打卡日期');
  await withPending('create-checkin', async () => {
    try {
      await api('/admin/checkins', { method: 'POST', body: JSON.stringify({ ...form, user_id: Number(form.user_id) }) });
      form.detail = ''; form.note = '';
      await loadCheckins();
      toast('打卡已补录，并写入审计记录');
    } catch (error) { toast(`补录失败：${error.message}`); }
  });
}

async function deleteAdminCheckin(item) {
  if (!await confirmDialog({ title: '纠正打卡记录', message: `确认撤销 ${item.logical_date} 的这条打卡吗？操作会保留审计记录。`, confirmLabel: '确认撤销', tone: 'danger' })) return;
  await withPending(`delete-checkin-${item.id}`, async () => {
    try { await api(`/admin/checkins/${item.id}`, { method: 'DELETE' }); await loadCheckins(); toast('记录已撤销'); }
    catch (error) { toast(`撤销失败：${error.message}`); }
  });
}

async function loadStatistics() {
  const params = new URLSearchParams(statisticsRange.value);
  const data = await api(`/admin/stats/completion?${params}`);
  completionStats.value = data.items || [];
}

async function loadGroupSettings() {
  const data = await api('/admin/group-settings');
  groupSettings.value = { group: data.group || {}, options: { ...groupSettings.value.options, ...(data.options || {}) } };
}

async function saveGroupSettings() {
  await withPending('save-group-settings', async () => {
    try { await api('/admin/group-settings', { method: 'PUT', body: JSON.stringify({ ...groupSettings.value.group, options: groupSettings.value.options }) }); await reloadApp(); toast('小组设置已保存'); }
    catch (error) { toast(`保存失败：${error.message}`); }
  });
}

async function loadAuditLogs() {
  const data = await api('/admin/audit-logs');
  auditLogs.value = data.items || [];
}

async function resetMemberPassword(member) {
  if (!await confirmDialog({ title: '重置临时密码', message: `确认重置 ${member.member_name || member.display_name} 的密码吗？该成员现有登录会失效。`, confirmLabel: '生成临时密码', tone: 'danger' })) return;
  await withPending(`reset-password-${member.user_id}`, async () => {
    try {
      const data = await api(`/admin/users/${member.user_id}/reset-password`, { method: 'POST' });
      await promptDialog({ title: '临时密码（仅显示这一次）', message: '请现在通过安全方式转交给成员；首次登录必须修改。', defaultValue: data.temporary_password, confirmLabel: '我已记录' });
    } catch (error) { toast(`重置失败：${error.message}`); }
  });
}

async function loadSuperData() {
  const [overview, accounts, allGroups] = await Promise.all([api('/super-admin/overview'), api('/super-admin/users'), api('/super-admin/groups')]);
  superOverview.value = overview;
  superUsers.value = accounts.users || [];
  platformGroups.value = allGroups.study_groups || [];
}

async function resetSuperUserPassword(account) {
  await withPending(`super-reset-${account.id}`, async () => {
    try {
      const data = await api(`/super-admin/users/${account.id}/reset-password`, { method: 'POST' });
      await promptDialog({ title: '临时密码（仅显示这一次）', message: `账号 @${account.username} 的旧登录已失效。`, defaultValue: data.temporary_password, confirmLabel: '我已记录' });
    } catch (error) { toast(`重置失败：${error.message}`); }
  });
}

async function mergeDuplicateUsers() {
  const primary = Number(mergeUsers.value.primary_user_id);
  const duplicate = Number(mergeUsers.value.duplicate_user_id);
  if (!primary || !duplicate || primary === duplicate) return toast('请选择两个不同的普通账号');
  const primaryAccount = superUsers.value.find(item => Number(item.id) === primary);
  const duplicateAccount = superUsers.value.find(item => Number(item.id) === duplicate);
  if (!await confirmDialog({ title: '合并重复账号', message: `保留 @${primaryAccount?.username}，把 @${duplicateAccount?.username} 的小组关系和历史数据并入后停用重复账号。若打卡冲突，系统会拒绝，不会覆盖记录。`, confirmLabel: '确认合并', tone: 'danger' })) return;
  await withPending('merge-users', async () => {
    try { await api('/super-admin/users/merge', { method: 'POST', body: JSON.stringify({ primary_user_id: primary, duplicate_user_id: duplicate }) }); mergeUsers.value = { primary_user_id: '', duplicate_user_id: '' }; await loadSuperData(); toast('重复账号已安全合并'); }
    catch (error) { toast(`合并失败：${error.message}`); }
  });
}

async function setGroupStatus(group, status) {
  const verb = status ? '恢复' : '归档';
  if (!await confirmDialog({ title: `${verb}小组`, message: `${verb}“${group.name}”？归档不会删除历史数据。`, confirmLabel: `确认${verb}`, tone: status ? 'default' : 'danger' })) return;
  await withPending(`group-status-${group.id}`, async () => {
    try { await api(`/super-admin/groups/${group.id}/status`, { method: 'PUT', body: JSON.stringify({ status }) }); await Promise.all([reloadApp(), loadSuperData()]); toast(`小组已${verb}`); }
    catch (error) { toast(`${verb}失败：${error.message}`); }
  });
}

async function chooseSection(id) {
  setAdminSection(id);
  if (id === 'overview') await loadAdminData();
  if (id === 'approvals') await loadRegistrationRequests();
  if (id === 'roster') await loadRoster();
  if (id === 'checkins') await loadCheckins();
  if (id === 'statistics') await loadStatistics();
  if (id === 'settings') await loadGroupSettings();
  if (id === 'audit') await loadAuditLogs();
  if (['accounts','operations'].includes(id)) await loadSuperData();
}

async function createStudyGroup() {
  const name = newGroupName.value.trim();
  const code = newGroupCode.value.trim().toLowerCase();
  if (!name || !/^[a-z0-9][a-z0-9-]{1,62}$/.test(code)) return toast('请填写小组名称；编码使用英文小写、数字或短横线');
  await withPending('create-group', async () => {
    try {
      const result = await api('/super-admin/groups', { method: 'POST', body: JSON.stringify({ code, name, description: newGroupDescription.value.trim() }) });
      newGroupCode.value = ''; newGroupName.value = ''; newGroupDescription.value = '';
      await reloadApp();
      await switchGroup(result.id);
      setAdminSection('overview');
      toast(`“${name}”已创建，现在可以导入名单和发布任务`);
    } catch (error) { toast(`创建失败：${error.message}`); }
  });
}

async function fetchRegistrationRequests() {
  const data = await api('/admin/registration-requests');
  registrationRequests.value = data.requests || [];
}

async function loadRegistrationRequests() {
  return withPending('load-registrations', async () => {
    try { await fetchRegistrationRequests(); }
    catch (error) { toast(`注册申请加载失败：${error.message}`); }
  });
}

async function approveRegistration(item) {
  const confirmed = await confirmDialog({ title: '通过注册申请', message: `确认开通 ${item.name}（@${item.username}）的账号吗？`, confirmLabel: '通过并开通' });
  if (!confirmed) return;
  await withPending(`approve-registration-${item.id}`, async () => {
    try {
      await api(`/admin/registration-requests/${item.id}/approve`, { method: 'POST' });
      await Promise.all([fetchRegistrationRequests(), reloadApp()]);
      toast('注册申请已通过，成员现在可以登录');
    } catch (error) { toast(`审批失败：${error.message}`); }
  });
}

async function rejectRegistration(item) {
  const reason = await promptDialog({ title: '拒绝注册申请', message: `请填写拒绝 ${item.name} 本次申请的原因。`, placeholder: '例如：姓名填写不一致', confirmLabel: '确认拒绝', tone: 'danger' });
  if (!reason) return;
  await withPending(`reject-registration-${item.id}`, async () => {
    try {
      await api(`/admin/registration-requests/${item.id}/reject`, { method: 'POST', body: JSON.stringify({ reason }) });
      await fetchRegistrationRequests();
      toast('注册申请已拒绝');
    } catch (error) { toast(`审批失败：${error.message}`); }
  });
}

async function loadRoster() {
  return withPending('load-roster', async () => {
    try { const data = await api('/super-admin/roster'); rosterEntries.value = data.entries || []; }
    catch (error) { toast(error.message); }
  });
}

async function uploadResource() {
  if (!canEditLearning.value) return toast('当前账号只有资源只读权限');
  await withPending('upload-resource', () => uploadLibraryFile(uploadInput.value, uploadCategory.value));
}

async function toggleResourceVisibility(item) {
  if (!canManageResources.value || !item?.resource_key) return;
  const visible = !item.visible_in_library;
  await withPending(`resource-visibility-${item.resource_key}`, async () => {
    try {
      await setResourceLibraryVisibility(item, visible);
      toast(visible ? '已显示在成员资料库' : '已从成员资料库隐藏');
    } catch (error) {
      toast(`保存失败：${error.message}`);
    }
  });
}

function chooseLibrarySection(key) { activeLibraryKey.value = key; activeLibraryFolder.value = ''; resourceSearch.value = ''; }
async function syncNASResources() {
  await withPending('sync-nas', async () => {
    try { await loadAdminData(true, true); toast('NAS 资源目录已重新扫描'); }
    catch (error) { toast(`同步失败：${error.message}`); }
  });
}
async function createResourceFolder() {
  if (!user.value?.is_super_admin) return toast('只有超级管理员可以修改全局 NAS 文件夹');
  const section = activeLibrarySection.value;
  if (!section?.managed_root) return toast('请选择 Book、Passage、PPT 或 Mentor 分类');
  if (!newFolderName.value.trim()) return toast('请输入文件夹名称');
  await withPending('create-folder', async () => {
    try {
      const data = await api('/admin/resource-folders', { method: 'POST', body: JSON.stringify({ root: section.managed_root, parent: activeLibraryFolder.value, name: newFolderName.value }) });
      newFolderName.value = ''; await loadAdminData(true, true); activeLibraryFolder.value = data.path || '';
      toast('文件夹已创建，可以在 NAS 中向该目录上传文件');
    } catch (error) { toast(`创建失败：${error.message}`); }
  });
}
async function renameResourceFolder() {
  if (!user.value?.is_super_admin) return toast('只有超级管理员可以修改全局 NAS 文件夹');
  const section = activeLibrarySection.value;
  if (!section?.managed_root || !activeLibraryFolder.value) return;
  const currentName = activeLibraryFolder.value.split('/').pop();
  const name = await promptDialog({ title: '重命名文件夹', message: '请输入新的文件夹名称。已发布任务中的旧路径可能需要重新挂载。', defaultValue: currentName, confirmLabel: '确认重命名' });
  if (!name || name === currentName) return;
  const warning = `即将把“${activeLibraryFolder.value}”重命名为“${name}”。\n\n已发布任务如果保存了旧文件路径，重命名后可能无法打开，需要重新选择资源。确认继续吗？`;
  if (!await confirmDialog({ title: '确认重命名文件夹', message: warning, confirmLabel: '继续重命名', tone: 'danger' })) return;
  await withPending('rename-folder', async () => {
    try {
      const data = await api('/admin/resource-folders', { method: 'PUT', body: JSON.stringify({ root: section.managed_root, path: activeLibraryFolder.value, name }) });
      await loadAdminData(true, true); activeLibraryFolder.value = data.path || '';
      toast('文件夹已重命名，请检查引用该目录的已发布任务');
    } catch (error) { toast(`重命名失败：${error.message}`); }
  });
}

async function previewRoster() {
  const file = rosterInput.value?.files?.[0]; if (!file) return toast('请先选择 Excel 文件');
  await withPending('preview-roster', async () => {
    try {
      const body = new FormData(); body.append('file', file);
      const res = await fetch('/api/super-admin/roster/preview', { method: 'POST', headers: { Authorization: `Bearer ${localStorage.getItem('agp_token') || ''}` }, body });
      rosterPreview.value = await res.json(); if (!res.ok) toast(rosterPreview.value.error || '名单解析失败');
    } catch (error) { rosterPreview.value = null; toast(`名单解析失败：${error.message}`); }
  });
}

async function importRoster() {
  const file = rosterInput.value?.files?.[0]; if (!file || !rosterPreview.value?.row_count) return;
  if (!await confirmDialog({ title: '同步报名名单', message: `确认将预览中的 ${rosterPreview.value.row_count} 个名单席位同步到系统吗？`, confirmLabel: '确认同步' })) return;
  await withPending('import-roster', async () => {
    try {
      const body = new FormData(); body.append('file', file);
      const res = await fetch('/api/super-admin/roster/import', { method: 'POST', headers: { Authorization: `Bearer ${localStorage.getItem('agp_token') || ''}` }, body });
      const data = await res.json(); if (!res.ok) return toast(data.error || '名单同步失败');
      rosterPreview.value = null; toast(`已同步 ${data.imported} 条名单`);
      const refreshed = await api('/super-admin/roster'); rosterEntries.value = refreshed.entries || [];
    } catch (error) { toast(`名单同步失败：${error.message}`); }
  });
}

async function addRosterEntry() {
  await withPending('add-roster', async () => {
    try {
      await api('/super-admin/roster/entries', { method: 'POST', body: JSON.stringify({ name: rosterName.value, group_id: Number(rosterGroupID.value), is_leader: rosterLeader.value, is_minor: rosterMinor.value, status: 1 }) });
      rosterName.value = ''; rosterGroupID.value = ''; rosterLeader.value = false; rosterMinor.value = false;
      toast('名单席位已添加');
      const refreshed = await api('/super-admin/roster'); rosterEntries.value = refreshed.entries || [];
    } catch (error) { toast(error.message); }
  });
}

async function runExport(path, name, message) {
  await withPending(`export-${name}`, async () => {
    try { await downloadAdminExport(path, name, message); }
    catch (error) { toast(error.message); }
  });
}
async function runWeeksImport() {
  if (!studyWeeksInput.value?.files?.[0]) return toast('请先选择任务计划 Excel');
  if (!await confirmDialog({ title: '导入任务计划', message: `任务计划将导入“${activeGroupName.value}”，确认继续吗？`, confirmLabel: '开始导入' })) return;
  await withPending('import-weeks', async () => {
    try { await importStudyWeeksExcel(studyWeeksInput.value); await reloadApp(); await loadAdminData(true); }
    catch (error) { toast(error.message); }
  });
}
async function runBackupImport() {
  if (!backupInput.value?.files?.[0]) return toast('请先选择完整备份 JSON');
  if (!await confirmDialog({ title: '恢复完整备份', message: `完整备份将写入“${activeGroupName.value}”，并可能替换当前任务和打卡数据。`, confirmLabel: '进入最后确认', tone: 'danger' })) return;
  if (!await confirmDialog({ title: '最后确认', message: `立即恢复“${activeGroupName.value}”的完整备份？此操作无法在网页中撤销。`, confirmLabel: '立即恢复', tone: 'danger' })) return;
  await withPending('import-backup', async () => {
    try {
      await importLocalBackupJSON(backupInput.value);
      await reloadApp();
      await loadAdminData(true);
      toast('完整备份已恢复，页面数据已重新载入');
    } catch (error) { toast(error.message); }
  });
}

function normalizedName(value) { return String(value || '').trim().replace(/\s+/g, '').toLocaleLowerCase(); }
function titleText(value) { return Array.isArray(value) ? value.filter(Boolean).join(' / ') : String(value || ''); }
function boolValue(value, fallback = true) { return typeof value === 'boolean' ? value : fallback; }
function legacyBinding(item, fallbackType = 'pdf') {
  if (!item) return null;
  if (typeof item === 'string') return { title: item, url: '', type: fallbackType, asset_id: 0 };
  return { title: item.title || '', url: item.url || '', type: item.type || fallbackType, asset_id: 0 };
}

function currentMemberMap() {
  const result = new Map();
  for (const member of members.value) {
    const profile = {
      username: member.username,
      display_name: member.member_name || member.display_name || member.username,
      name_pinyin: member.name_pinyin || member.username,
      roles: Array.isArray(member.roles) ? member.roles : [],
    };
    for (const name of [member.member_name, member.display_name, member.username, member.name_pinyin]) {
      if (name) result.set(normalizedName(name), profile);
    }
  }
  return result;
}

function convertLegacyWeek(week) {
  const videoItems = Array.isArray(week.videos) ? week.videos.map((item) => legacyBinding(item, 'video')).filter(Boolean) : [];
  if (!videoItems.length && (week.video || week.url)) videoItems.push({ title: week.video || '本周视频', url: week.url || '', type: 'video', asset_id: 0 });
  return {
    start_date: week.start || week.start_date || '', end_date: week.end || week.end_date || '', title: titleText(week.title),
    verse_ref: week.verse || week.verse_ref || '', recite_text: week.reciteText || week.recite_text || '',
    book_enabled: boolValue(week.book_enabled), video_enabled: boolValue(week.video_enabled),
    verse_enabled: boolValue(week.verse_enabled), outline_enabled: boolValue(week.outline_enabled),
    readings: (Array.isArray(week.readings) ? week.readings : []).map((item) => legacyBinding(item, 'pdf')).filter(Boolean),
    videos: videoItems,
    outline: week.outlineImage ? { title: '提纲图片', url: week.outlineImage, type: 'image', asset_id: 0 } : legacyBinding(week.outline, 'image') || { title: '', url: '', type: 'image', asset_id: 0 },
  };
}

function recordRows(record, username) {
  const common = {
    username, logical_date: record.logical_date || '', checkin_time: record.checkin_time || '',
    part: record.part || '', detail: record.detail || '', note: record.note || '',
    is_retro: record.is_retro === true || record.is_retro === 1 || ['1','true','yes'].includes(String(record.is_retro || '').toLowerCase()),
  };
  const rows = [];
  const add = (taskType) => rows.push({ ...common, task_type: taskType, detail: common.detail || taskType });
  if (String(record.daily || '').toLowerCase() === 'done') add('daily_devotion');
  if (String(record.book || '').toLowerCase() === 'done') add('weekly_book');
  if (String(record.video || '').toLowerCase() === 'done') add('weekly_video');
  if (String(record.verse || '').toLowerCase() === 'done') add('weekly_verse');
  if (record.kind === 'reflection') add('reflection');
  if (record.kind === 'recite_exam') add('recite_exam');
  return rows;
}

async function previewLegacyRestore() {
  await withPending('preview-legacy', async () => {
    try {
      const configFile = legacyConfigInput.value?.files?.[0];
      if (!configFile) return toast('请先选择旧网站的 config.json');
      const config = JSON.parse(await configFile.text());
      const recordsFile = legacyRecordsInput.value?.files?.[0];
      const records = recordsFile ? JSON.parse(await recordsFile.text()) : [];
      if (!Array.isArray(config.weekly_schedule)) throw new Error('config.json 中没有 weekly_schedule');
      if (!Array.isArray(records)) throw new Error('打卡记录 JSON 必须是数组');
      const memberMap = currentMemberMap();
      const unmatched = new Set();
      const checkins = [];
      for (const record of records) {
        const member = memberMap.get(normalizedName(record.name));
        if (!member?.username) { if (record.name) unmatched.add(record.name); continue; }
        checkins.push(...recordRows(record, member.username));
      }
      const uniqueMembers = [...new Map([...memberMap.values()].map((item) => [item.username, item])).values()];
      legacyPreview.value = {
        payload: {
          version: 1, exported_at: new Date().toISOString(), group: { id: currentGroupID.value },
          settings: { site_info: config.site_info || {}, task_sections: config.task_sections || {}, mounted_files: config.mounted_files || {}, class_rep_shares: config.class_rep_shares || [] },
          members: uniqueMembers, weeks: config.weekly_schedule.map(convertLegacyWeek), checkins, feedbacks: [], assets: [],
        },
        weeks: config.weekly_schedule.length, sourceRecords: records.length, checkins: checkins.length, unmatched: [...unmatched],
      };
    } catch (error) { legacyPreview.value = null; toast(`解析失败：${error.message}`); }
  });
}

async function confirmLegacyRestore() {
  if (!legacyPreview.value?.payload) return;
  const message = `将用旧网站数据替换“${activeGroupName.value}”的任务设置和打卡记录。\n\n任务：${legacyPreview.value.weeks} 周\n打卡明细：${legacyPreview.value.checkins} 条\n\n确认进入最后确认吗？`;
  if (!await confirmDialog({ title: '恢复旧网站数据', message, confirmLabel: '进入最后确认', tone: 'danger' })) return;
  if (!await confirmDialog({ title: '最后确认', message: `立即覆盖“${activeGroupName.value}”的任务设置和打卡记录？此操作无法在网页中撤销。`, confirmLabel: '立即覆盖', tone: 'danger' })) return;
  await withPending('restore-legacy', async () => {
    try {
      await api('/admin/imports/local-backup', { method: 'POST', body: JSON.stringify(legacyPreview.value.payload), timeout: 15 * 60 * 1000 });
      legacyPreview.value = null;
      if (legacyConfigInput.value) legacyConfigInput.value.value = '';
      if (legacyRecordsInput.value) legacyRecordsInput.value.value = '';
      await reloadApp();
      await loadAdminData(true);
      toast('旧网站任务设置和打卡记录已恢复，页面数据已重新载入');
    } catch (error) { toast(`恢复失败：${error.message}`); }
  });
}

function selectResourceForTask(item) {
  if (!canEditLearning.value) return toast('当前账号只能预览资源，不能修改任务');
  if (!weekDraft.value) createWeekDraft();
  const value = librarySelectionValue(item);
  if (!value) return toast('该资源没有可用地址');
  const isImage = item.type === 'image' || item.category === 'outline';
  const isVideo = item.type === 'video' || item.category === 'video';
  if (isImage) applyOutlineSelection(value);
  else {
    const kind = isVideo ? 'videos' : 'readings';
    const list = weekDraft.value?.[kind] || [];
    let index = list.findIndex((entry) => !entry.title && !entry.url && !entry.asset_id);
    if (index < 0) { addWeekBinding(kind); index = list.length; }
    applyBindingSelection(kind, index, value);
  }
  setAdminSection('learning');
  toast('资源已选入当前任务，请确认周次后保存');
}

onMounted(() => {
  if (user.value?.is_super_admin && !currentGroupID.value) setAdminSection('groups');
  else if (!['groups','overview','learning','approvals','members','checkins','library','statistics','settings','audit','roster','accounts','operations','data'].includes(adminSection.value)) setAdminSection('overview');
  fetchRegistrationRequests().catch(() => {});
  adminNavMobileQuery = window.matchMedia('(max-width: 900px)');
  if (adminNavMobileQuery.addEventListener) adminNavMobileQuery.addEventListener('change', handleAdminNavViewportChange);
  else adminNavMobileQuery.addListener(handleAdminNavViewportChange);
  if (adminNavMobileQuery.matches) centerActiveAdminSection('auto');
  loadAdminData();
});

onBeforeUnmount(() => {
  if (!adminNavMobileQuery) return;
  if (adminNavMobileQuery.removeEventListener) adminNavMobileQuery.removeEventListener('change', handleAdminNavViewportChange);
  else adminNavMobileQuery.removeListener(handleAdminNavViewportChange);
});
</script>

<template>
  <div class="ios-admin-layout">
    <aside ref="adminNav" class="ios-admin-nav" role="navigation" aria-label="管理功能导航">
      <div class="ios-admin-nav-title"><small>ADMIN CONSOLE</small><b>小组管理</b></div>
      <button v-for="section in sections" :key="section.id" :class="{ active: adminSection === section.id }" :aria-current="adminSection === section.id ? 'page' : undefined" type="button" @click="chooseSection(section.id)">
        <AppIcon :name="section.icon" :size="19" /><span>{{ section.label }}</span><AppIcon class="admin-nav-chevron" name="chevron" :size="16" />
      </button>
    </aside>

    <main class="ios-admin-main">
      <div v-if="isReadOnlyAdmin && ['learning','library','data'].includes(adminSection)" class="ios-roster-match" role="status">
        <AppIcon name="lock" :size="18" />
        <div><b>当前账号为只读管理权限</b><small>请联系组长或超级管理员授予小组管理员权限后再修改任务。</small></div>
      </div>
      <div v-if="adminLoading && !['overview','members','roster','data'].includes(adminSection)" class="ios-loading">正在载入管理数据…</div>

      <section v-else-if="adminSection === 'groups' && user?.is_super_admin" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>PLATFORM</small><h1>小组管理</h1><p>新增小组只需在这里创建，不再部署新的容器或网站。</p></div></header>
        <div class="ios-panel-grid group-management-grid">
          <article class="ios-panel"><div class="panel-heading"><div><h2>创建新小组</h2><small>创建后即可导入 Excel 名单并发布任务</small></div></div><div class="ios-stack"><label><span>小组名称</span><input v-model.trim="newGroupName" placeholder="例如：2026 生命季一组" /></label><label><span>小组编码</span><input v-model.trim="newGroupCode" placeholder="例如：life-2026-a" @keydown.enter.prevent="createStudyGroup" /></label><label><span>说明（选填）</span><textarea v-model.trim="newGroupDescription" rows="3" placeholder="成员能够理解的简短说明"></textarea></label><button :disabled="!newGroupName || !newGroupCode || !!pendingAction" type="button" @click="createStudyGroup">{{ isPending('create-group') ? '创建中…' : '创建并进入小组' }}</button></div></article>
          <article class="ios-panel"><div class="panel-heading"><div><h2>现有小组</h2><small>共 {{ groups.length }} 个正常运行的小组</small></div></div><div v-if="groups.length" class="group-card-list"><button v-for="group in groups" :key="group.id" :class="{ active: Number(group.id) === Number(currentGroupID) }" type="button" @click="switchGroup(group.id)"><span class="member-avatar-fallback">{{ group.name.slice(0,1) }}</span><div><b>{{ group.name }}</b><small>{{ group.code }}</small></div><em>{{ Number(group.id) === Number(currentGroupID) ? '当前小组' : '进入管理' }}</em></button></div><div v-else class="ios-empty"><AppIcon name="users" :size="28" /><b>还没有小组</b><span>填写左侧三个字段即可创建第一个小组。</span></div></article>
        </div>
      </section>

      <section v-else-if="adminSection === 'overview'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>OVERVIEW</small><h1>管理概览</h1><p>今天需要关注的内容集中在这里。</p></div></header>
        <div class="ios-metric-grid">
          <button type="button" @click="chooseSection('members')"><span class="metric-icon blue"><AppIcon name="users" /></span><b>{{ members.length }}</b><small>当前组成员</small></button>
          <button type="button" @click="chooseSection('learning')"><span class="metric-icon purple"><AppIcon name="calendar" /></span><b>{{ weeks.length }}</b><small>周任务计划</small></button>
          <button type="button" @click="chooseSection('library')"><span class="metric-icon green"><AppIcon name="library" /></span><b>{{ libraryItems.length }}</b><small>可用学习资源</small></button>
          <button type="button" @click="chooseSection('members')"><span class="metric-icon orange"><AppIcon name="check" /></span><b>{{ completedProfiles }}</b><small>已启用账号</small></button>
        </div>
        <div class="ios-panel-grid">
          <article class="ios-panel current-plan-panel">
            <div class="panel-heading"><div><small>CURRENT PLAN</small><h2>当前周任务</h2></div><button class="ios-text-button" type="button" @click="chooseSection('learning')">管理计划</button></div>
            <div v-if="currentWeek" class="current-plan-card"><span class="ios-status active">{{ weekStatus(currentWeek) }}</span><h3>{{ currentWeek.title || '未命名周任务' }}</h3><p>{{ currentWeek.start }} — {{ currentWeek.end }}</p><div class="plan-tags"><span v-if="currentWeek.book_enabled !== false">读物</span><span v-if="currentWeek.video_enabled !== false">视频</span><span v-if="currentWeek.verse_enabled !== false">背经</span></div></div>
            <div v-else class="ios-empty"><AppIcon name="calendar" :size="28" /><b>还没有周任务</b><span>创建第一周计划后，成员即可开始打卡。</span><button :disabled="!canEditLearning" type="button" @click="chooseSection('learning'); createWeekDraft()">创建周任务</button></div>
          </article>
          <article class="ios-panel quick-actions-panel">
            <div class="panel-heading"><div><small>QUICK ACTIONS</small><h2>常用操作</h2></div></div>
            <button :disabled="!canEditLearning" type="button" @click="chooseSection('learning'); createWeekDraft()"><span><AppIcon name="plus" /></span><div><b>新建周任务</b><small>安排读物、视频与背经</small></div><AppIcon name="chevron" /></button>
            <button type="button" @click="chooseSection('library')"><span><AppIcon name="upload" /></span><div><b>上传学习资料</b><small>添加 PDF、图片或 Markdown</small></div><AppIcon name="chevron" /></button>
            <button type="button" @click="chooseSection('members')"><span><AppIcon name="users" /></span><div><b>查看成员权限</b><small>管理本组管理员</small></div><AppIcon name="chevron" /></button>
          </article>
        </div>
      </section>

      <section v-else-if="adminSection === 'learning'" class="ios-admin-page">
        <header class="ios-page-heading heading-with-action"><div><small>THIS WEEK</small><h1>本周任务发布</h1><p>周期、内容、预览都在这里完成，不需要理解任务代码或资源路径。</p></div><button :disabled="!canEditLearning || !!pendingAction" type="button" @click="prepareNextWeek(false)"><AppIcon name="plus" :size="18" />复制到下一周</button></header>
        <div class="planner-progress"><div><b>已完成 {{ taskProgress.done }}/{{ taskProgress.total }} 项</b><span>{{ weekDraft?.publication_status === 'published' ? '当前版本已发布' : '草稿不会显示给成员' }}</span></div><progress :value="taskProgress.done" :max="taskProgress.total"></progress></div>
        <div class="simple-planner-steps" aria-label="发布步骤"><button :class="{ active: plannerStep === 1 }" type="button" @click="plannerStep = 1"><b>1</b>选择周期</button><button :class="{ active: plannerStep === 2 }" :disabled="!weekDraft?.start || !weekDraft?.end" type="button" @click="plannerStep = 2"><b>2</b>填写内容</button><button :class="{ active: plannerStep === 3 }" :disabled="publishErrors.length > 0" type="button" @click="previewWeekDraft"><b>3</b>预览发布</button></div>
        <div class="ios-planner-layout">
          <aside class="ios-week-list">
            <div class="list-caption">周任务 · {{ weeks.length }}</div>
            <button v-for="week in weeks" :key="week.id" :class="{ active: Number(weekDraft?.id) === Number(week.id) }" type="button" @click="chooseWeek(week.id)"><span class="ios-status" :class="{ active: week.publication_status === 'published' }">{{ week.publication_status === 'published' ? weekStatus(week) : '草稿' }}</span><b>{{ week.title || '未命名草稿' }}</b><small>{{ week.start }} — {{ week.end }}</small></button>
            <button class="empty-week-button" :disabled="!canEditLearning" type="button" @click="prepareNextWeek(true)">＋ 使用空白周</button>
          </aside>

          <div class="ios-planner-content">
            <article v-if="weekDraft" class="ios-panel ios-week-editor">
              <template v-if="plannerStep === 1">
                <div class="panel-heading"><div><small>第 1 步</small><h2>确认学习周期</h2></div><span class="ios-info-chip">新建时默认复制上一周</span></div>
                <div class="ios-form-grid">
                  <label class="span-2"><span>本周主题</span><input :disabled="!canEditLearning" :value="weekDraft.title || ''" placeholder="例如：马可福音（上）" @input="updateWeekDraftField('title', $event.target.value)" /></label>
                  <label><span>开始日期</span><AppDatePicker :disabled="!canEditLearning" :model-value="weekDraft.start || ''" label="选择开始日期" @update:model-value="updateWeekDraftField('start', $event)" /></label>
                  <label><span>结束日期</span><AppDatePicker :disabled="!canEditLearning" :model-value="weekDraft.end || ''" label="选择结束日期" @update:model-value="updateWeekDraftField('end', $event)" /></label>
                </div>
                <div v-if="validateWeekDates()" class="ios-form-error" role="alert">{{ validateWeekDates() }}</div>
                <div class="planner-next"><button :disabled="!!validateWeekDates()" type="button" @click="plannerStep = 2">下一步：填写内容</button></div>
              </template>

              <template v-else-if="plannerStep === 2">
                <div class="panel-heading"><div><small>第 2 步</small><h2>填写学习内容</h2><p>不用的项目直接关闭；文件可以在任务卡里上传。</p></div><span class="ios-info-chip">{{ taskProgress.done }}/{{ taskProgress.total }} 项已就绪</span></div>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="dailyDevotion.enabled !== false" @change="updateLearningValue(['task_sections','daily','devotion','enabled'],$event.target.checked)" /><b>每日灵修</b></label><span class="task-ready">{{ dailyDevotion.enabled === false || dailyDevotion.title || dailyDevotion.path ? '已就绪' : '待填写' }}</span></header><div v-if="dailyDevotion.enabled !== false" class="simple-task-fields ios-form-grid"><label><span>栏目名称</span><input :value="dailyDevotion.title || ''" placeholder="例如：每日灵修" @input="updateLearningValue(['task_sections','daily','devotion','title'],$event.target.value)" /></label><label><span>阅读按钮</span><input :value="dailyDevotion.button_label || ''" placeholder="阅读今日内容" @input="updateLearningValue(['task_sections','daily','devotion','button_label'],$event.target.value)" /></label></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="dailyScripture.enabled !== false" @change="updateLearningValue(['task_sections','daily','scripture','enabled'],$event.target.checked)" /><b>每日读经</b></label><span class="task-ready">{{ dailyScripture.enabled === false || dailyScripture.label || dailyScripture.book ? '已就绪' : '待填写' }}</span></header><div v-if="dailyScripture.enabled !== false" class="simple-task-fields ios-form-grid"><label><span>栏目名称</span><input :value="dailyScripture.label || ''" placeholder="例如：每日读经" @input="updateLearningValue(['task_sections','daily','scripture','label'],$event.target.value)" /></label><label><span>书卷名称</span><input :value="dailyScripture.book || ''" placeholder="例如：马可福音" @input="updateLearningValue(['task_sections','daily','scripture','book'],$event.target.value)" /></label><label><span>起始日期</span><AppDatePicker :model-value="dailyScripture.start_date || ''" label="读经起始日期" @update:model-value="updateLearningValue(['task_sections','daily','scripture','start_date'],$event)" /></label><label><span>起始章</span><input type="number" min="1" :value="dailyScripture.start_chapter || 1" @input="updateLearningValue(['task_sections','daily','scripture','start_chapter'],Number($event.target.value || 1))" /></label></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="weekDraft.book_enabled !== false" @change="updateWeekDraftField('book_enabled',$event.target.checked)" /><b>周读物</b></label><button v-if="weekDraft.book_enabled !== false" class="ios-text-button" :disabled="!canEditLearning" type="button" @click="addWeekBinding('readings')">＋ 添加一项</button></header><div v-if="weekDraft.book_enabled !== false" class="simple-task-fields"><div class="task-binding-card" v-for="(item,index) in weekDraft.readings || []" :key="`r-${index}`"><input :disabled="!canEditLearning" :value="item.title || ''" :aria-label="`第 ${index + 1} 项读物标题`" placeholder="读物标题" @input="updateWeekBinding('readings',index,'title',$event.target.value)" /><select :disabled="!canEditLearning" :value="librarySelectionValue(item)" @change="applyBindingSelection('readings',index,$event.target.value)"><option value="">从资料库选择</option><option v-for="option in readingOptions" :key="librarySelectionValue(option)" :value="librarySelectionValue(option)">{{ optionText(option) }}</option></select><label class="inline-upload"><input type="file" accept=".pdf,.md" :disabled="!!pendingAction" @change="uploadTaskFile('readings',index,$event)" /><span>{{ isPending(`task-upload-readings-${index}`) ? '上传中…' : '直接上传文件' }}</span></label><button class="icon-danger" :disabled="!canEditLearning" type="button" aria-label="移除读物" @click="removeWeekBinding('readings',index)">×</button></div></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="weekDraft.video_enabled !== false" @change="updateWeekDraftField('video_enabled',$event.target.checked)" /><b>本周视频</b></label><button v-if="weekDraft.video_enabled !== false" class="ios-text-button" :disabled="!canEditLearning" type="button" @click="addWeekBinding('videos')">＋ 添加一项</button></header><div v-if="weekDraft.video_enabled !== false" class="simple-task-fields"><div class="task-binding-card" v-for="(item,index) in weekDraft.videos || []" :key="`v-${index}`"><input :disabled="!canEditLearning" :value="item.title || ''" placeholder="视频标题" @input="updateWeekBinding('videos',index,'title',$event.target.value)" /><input :disabled="!canEditLearning" :value="item.url || ''" placeholder="视频链接（也可直接上传）" @input="updateWeekBinding('videos',index,'url',$event.target.value)" /><label class="inline-upload"><input type="file" accept="video/*" :disabled="!!pendingAction" @change="uploadTaskFile('videos',index,$event)" /><span>{{ isPending(`task-upload-videos-${index}`) ? '上传中…' : '直接上传视频' }}</span></label><button class="icon-danger" :disabled="!canEditLearning" type="button" aria-label="移除视频" @click="removeWeekBinding('videos',index)">×</button></div></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="weekDraft.verse_enabled !== false" @change="updateWeekDraftField('verse_enabled',$event.target.checked)" /><b>背经与默写</b></label></header><div v-if="weekDraft.verse_enabled !== false" class="simple-task-fields ios-form-grid"><label><span>经文范围</span><input :disabled="!canEditLearning" :value="weekDraft.verse_ref || ''" placeholder="例如：罗马书 8:1-5" @input="updateWeekDraftField('verse_ref',$event.target.value)" /></label><label class="span-2"><span>经文原文</span><textarea :disabled="!canEditLearning" rows="5" :value="weekDraft.recite_text || ''" placeholder="粘贴本周需要背诵和默写的经文" @input="updateWeekDraftField('recite_text',$event.target.value)"></textarea></label></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="weekDraft.outline_enabled !== false" @change="updateWeekDraftField('outline_enabled',$event.target.checked)" /><b>提纲图片</b></label></header><div v-if="weekDraft.outline_enabled !== false" class="simple-task-fields"><select :disabled="!canEditLearning" :value="librarySelectionValue(weekDraft.outline)" @change="applyOutlineSelection($event.target.value)"><option value="">从资料库选择</option><option v-for="item in outlineOptions" :key="librarySelectionValue(item)" :value="librarySelectionValue(item)">{{ optionText(item) }}</option></select><label class="inline-upload wide"><input type="file" accept="image/*" :disabled="!!pendingAction" @change="uploadTaskFile('outline',0,$event)" /><span>{{ isPending('task-upload-outline-0') ? '上传中…' : '直接上传提纲图片' }}</span></label></div></section>
                <section class="simple-task-card"><header><label><input :disabled="!canEditLearning" type="checkbox" :checked="shareSettings.enabled !== false" @change="updateLearningValue(['task_sections','share','enabled'],$event.target.checked)" /><b>得着分享</b></label></header><div v-if="shareSettings.enabled !== false" class="simple-task-fields"><label><span>成员端名称</span><input :value="shareSettings.label || ''" placeholder="例如：写下今天的得着" @input="updateLearningValue(['task_sections','share','label'],$event.target.value)" /></label></div></section>
                <div class="planner-next split"><button class="ios-secondary-button" type="button" @click="plannerStep = 1">上一步</button><button :disabled="publishErrors.length > 0" type="button" @click="previewWeekDraft">预览成员页面</button></div>
              </template>

              <template v-else>
                <div class="panel-heading"><div><small>第 3 步</small><h2>成员端效果预览</h2><p>确认周期和任务开关，发布后成员立即可见。</p></div><span class="ios-status" :class="{ active: weekDraft.publication_status === 'published' }">{{ weekDraft.publication_status === 'published' ? '已发布' : '草稿' }}</span></div>
                <div class="phone-task-preview"><div class="phone-preview-top"><small>{{ activeGroupName }}</small><b>{{ weekDraft.title || '未命名本周任务' }}</b><span>{{ weekDraft.start }} — {{ weekDraft.end }}</span></div><div class="phone-preview-list"><div v-if="dailyDevotion.enabled !== false"><span>01</span><b>{{ dailyDevotion.title || '每日灵修' }}</b><em>每天</em></div><div v-if="dailyScripture.enabled !== false"><span>02</span><b>{{ dailyScripture.label || '每日读经' }}</b><em>{{ dailyScripture.book || '按日阅读' }}</em></div><div v-if="weekDraft.book_enabled !== false"><span>03</span><b>周读物</b><em>{{ (weekDraft.readings || []).filter(i => i.title || i.asset_id || i.url).length }} 项</em></div><div v-if="weekDraft.video_enabled !== false"><span>04</span><b>本周视频</b><em>{{ (weekDraft.videos || []).filter(i => i.title || i.asset_id || i.url).length }} 项</em></div><div v-if="weekDraft.verse_enabled !== false"><span>05</span><b>背经与默写</b><em>{{ weekDraft.verse_ref }}</em></div><div v-if="shareSettings.enabled !== false"><span>06</span><b>{{ shareSettings.label || '得着分享' }}</b><em>选填</em></div></div></div>
                <div v-if="publishErrors.length" class="publish-error-list"><b>发布前还需要处理：</b><span v-for="error in publishErrors" :key="error">{{ error }}</span></div>
                <div class="planner-next split"><button class="ios-secondary-button" type="button" @click="plannerStep = 2">返回修改</button><button :disabled="publishErrors.length > 0 || !!pendingAction" type="button" @click="saveWeek('published')">{{ isPending('save-week-published') ? '发布中…' : (weekDraft.publication_status === 'published' ? '确认重新发布' : '确认发布') }}</button></div>
              </template>

              <footer class="ios-editor-actions compact-actions"><button v-if="weekDraft.id && weekDraft.publication_status !== 'published'" class="ios-danger-button" :disabled="!canEditLearning || !!pendingAction" type="button" @click="deleteWeekWithPending">{{ isPending('delete-week') ? '删除中…' : '删除草稿' }}</button><button v-if="weekDraft.publication_status !== 'published'" class="ios-secondary-button" :disabled="!canEditLearning || !!pendingAction || !!validateWeekDates()" type="button" @click="saveWeek('draft')">{{ isPending('save-week-draft') ? '保存中…' : '保存草稿' }}</button></footer>
            </article>
          </div>
        </div>
      </section>

      <section v-else-if="adminSection === 'members'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>PEOPLE</small><h1>成员与权限</h1><p>成员通过报名名单注册；这里只管理本组权限。</p></div></header>
        <div class="ios-member-toolbar"><span>共 {{ members.length }} 位成员</span><span class="ios-info-chip">组长 {{ members.filter(m => m.roles?.includes('group_leader')).length }}</span><span class="ios-info-chip">管理员 {{ members.filter(m => m.roles?.includes('group_admin')).length }}</span></div>
        <div class="ios-member-list"><article v-for="member in members" :key="member.member_id" class="ios-member-row"><img v-if="member.avatar_url" :src="member.avatar_url" :alt="member.member_name" /><span v-else class="member-avatar-fallback">{{ (member.member_name || member.display_name || '?').slice(0,1) }}</span><div class="member-copy"><b>{{ member.member_name || member.display_name }}</b><small>@{{ member.username }}</small></div><span class="ios-role" :class="roleLabel(member)">{{ roleLabel(member) }}</span><div class="member-row-actions"><button v-if="member.user_id !== user?.id && !member.is_super_admin" class="ios-text-button" :disabled="!!pendingAction" type="button" @click="resetMemberPassword(member)">重置密码</button><button v-if="!member.is_super_admin && !member.roles?.includes('group_leader')" class="ios-text-button" :disabled="!!pendingAction" type="button" @click="changeMemberAdmin(member)">{{ isPending(`member-role-${member.member_id}`) ? '处理中…' : (member.roles?.includes('group_admin') ? '取消管理员' : '设为管理员') }}</button><button v-if="member.user_id !== user?.id && !member.is_super_admin && !member.roles?.includes('group_leader')" class="ios-text-danger" :disabled="!!pendingAction" type="button" @click="removeMemberWithPending(member)">{{ isPending(`remove-member-${member.member_id}`) ? '移出中…' : '移出本组' }}</button></div></article></div>
      </section>

      <section v-else-if="adminSection === 'checkins'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>CHECK-INS</small><h1>打卡记录</h1><p>筛选、补录和纠错集中在一页，所有修改都会留下审计记录。</p></div></header>
        <div class="ios-panel-grid">
          <article class="ios-panel"><div class="panel-heading"><div><h2>筛选记录</h2><small>最多显示最近 200 条</small></div></div><div class="ios-form-grid"><label><span>开始日期</span><input v-model="checkinFilters.from" type="date" /></label><label><span>结束日期</span><input v-model="checkinFilters.to" type="date" /></label><label class="span-2"><span>成员</span><select v-model="checkinFilters.user_id"><option value="">全部成员</option><option v-for="member in members" :key="member.user_id" :value="member.user_id">{{ member.member_name || member.display_name }}</option></select></label><button type="button" :disabled="!!pendingAction" @click="loadCheckins">查询</button></div></article>
          <article class="ios-panel"><div class="panel-heading"><div><h2>管理员补录</h2><small>可绕过成员补卡期限，但必须填写准确日期</small></div></div><div class="ios-form-grid"><label><span>成员</span><select v-model="checkinForm.user_id"><option value="">选择成员</option><option v-for="member in members" :key="member.user_id" :value="member.user_id">{{ member.member_name || member.display_name }}</option></select></label><label><span>日期</span><input v-model="checkinForm.logical_date" type="date" /></label><label><span>类型</span><select v-model="checkinForm.task_type"><option value="daily_devotion">每日灵修</option><option value="weekly_book">周读物</option><option value="weekly_video">本周视频</option><option value="weekly_verse">背经默写</option></select></label><label><span>说明</span><input v-model.trim="checkinForm.detail" placeholder="选填" /></label><label class="span-2"><span>得着（选填）</span><textarea v-model.trim="checkinForm.note" rows="2"></textarea></label><button class="span-2" type="button" :disabled="!checkinForm.user_id || !!pendingAction" @click="createAdminCheckin">确认补录</button></div></article>
        </div>
        <div class="ios-member-list"><article v-for="item in checkinRows" :key="item.id" class="ios-member-row"><span class="member-avatar-fallback">{{ item.logical_date.slice(8) }}</span><div class="member-copy"><b>{{ members.find(m => Number(m.user_id) === Number(item.user_id))?.member_name || `成员 #${item.user_id}` }}</b><small>{{ item.logical_date }} · {{ item.task_type }}<template v-if="item.detail"> · {{ item.detail }}</template></small></div><button class="ios-text-danger" type="button" :disabled="!!pendingAction" @click="deleteAdminCheckin(item)">撤销纠错</button></article><div v-if="!checkinRows.length" class="ios-empty">当前筛选范围没有打卡记录</div></div>
      </section>

      <section v-else-if="adminSection === 'statistics'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>STATISTICS</small><h1>完成统计</h1><p>成员完成矩阵和未完成名单一眼可查。</p></div></header>
        <article class="ios-panel"><div class="ios-form-grid"><label><span>开始日期</span><input v-model="statisticsRange.from" type="date" /></label><label><span>结束日期</span><input v-model="statisticsRange.to" type="date" /></label><button type="button" :disabled="!!pendingAction" @click="loadStatistics">刷新统计</button></div></article>
        <div class="completion-table" role="table" aria-label="成员完成矩阵"><div class="completion-row completion-head" role="row"><b>成员</b><span>灵修</span><span>读物</span><span>视频</span><span>背经</span><strong>合计</strong></div><div v-for="item in completionStats" :key="item.user_id" class="completion-row" role="row"><b>{{ item.member_name }}</b><span>{{ item.daily_devotion }}</span><span>{{ item.weekly_book }}</span><span>{{ item.weekly_video }}</span><span>{{ item.weekly_verse }}</span><strong>{{ item.total }}</strong></div></div>
      </section>

      <section v-else-if="adminSection === 'settings'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>GROUP SETTINGS</small><h1>小组设置</h1><p>只保留实际会用到的文案、补卡和隐私选项。</p></div></header>
        <article class="ios-panel"><div class="ios-form-grid"><label><span>小组名称</span><input v-model.trim="groupSettings.group.name" /></label><label><span>允许补卡天数（0–90）</span><input v-model.number="groupSettings.options.retro_days" type="number" min="0" max="90" /></label><label class="span-2"><span>小组说明</span><textarea v-model.trim="groupSettings.group.description" rows="2"></textarea></label><label><span>网站标题</span><input v-model.trim="groupSettings.options.site_title" /></label><label><span>首页提示</span><input v-model.trim="groupSettings.options.home_message" /></label></div><div class="ios-toggle-grid"><label><input v-model="groupSettings.options.show_group_summary" type="checkbox" />允许成员看小组概览</label><label><input v-model="groupSettings.options.show_member_status" type="checkbox" />显示具体成员完成状态</label><label><input v-model="groupSettings.options.show_reflections" type="checkbox" />显示成员主动设为组内可见的得着</label><label><input v-model="groupSettings.options.allow_member_ranking" type="checkbox" />允许组内成员排行</label></div><button type="button" :disabled="!!pendingAction" @click="saveGroupSettings">保存小组设置</button></article>
      </section>

      <section v-else-if="adminSection === 'audit'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>AUDIT</small><h1>审计记录</h1><p>管理员写操作不可在网页中删除。</p></div><button class="ios-secondary-button" type="button" @click="loadAuditLogs">刷新</button></header>
        <div class="ios-member-list"><article v-for="item in auditLogs" :key="item.id" class="ios-member-row"><span class="member-avatar-fallback">{{ item.id }}</span><div class="member-copy"><b>{{ item.action }}</b><small>{{ item.created_at }} · 操作者 #{{ item.actor_user_id }} · {{ item.target_type }} #{{ item.target_id }}</small></div></article><div v-if="!auditLogs.length" class="ios-empty">暂无管理操作记录</div></div>
      </section>

      <section v-else-if="adminSection === 'accounts' && user?.is_super_admin" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>ACCOUNTS</small><h1>全体账号</h1><p>查询账号并进行单人密码重置，不再使用全员批量重置。</p></div></header>
        <article class="ios-panel"><div class="panel-heading"><div><h2>合并重复账号</h2><small>保留主账号；发生打卡冲突时自动拒绝</small></div></div><div class="ios-form-grid"><label><span>保留账号</span><select v-model="mergeUsers.primary_user_id"><option value="">选择主账号</option><option v-for="account in superUsers.filter(item => !item.is_super_admin && item.status === 1)" :key="account.id" :value="account.id">{{ account.display_name }} (@{{ account.username }})</option></select></label><label><span>停用并合并</span><select v-model="mergeUsers.duplicate_user_id"><option value="">选择重复账号</option><option v-for="account in superUsers.filter(item => !item.is_super_admin && item.status === 1)" :key="account.id" :value="account.id">{{ account.display_name }} (@{{ account.username }})</option></select></label><button class="span-2 ios-danger-button" type="button" :disabled="!!pendingAction" @click="mergeDuplicateUsers">检查冲突并合并</button></div></article>
        <div class="ios-member-list"><article v-for="account in superUsers" :key="account.id" class="ios-member-row"><span class="member-avatar-fallback">{{ (account.display_name || account.username).slice(0,1) }}</span><div class="member-copy"><b>{{ account.display_name }}</b><small>@{{ account.username }} · {{ account.status === 1 ? '正常' : '已停用' }}</small></div><span v-if="account.is_super_admin" class="ios-role">超级管理员</span><button v-else class="ios-text-button" type="button" :disabled="!!pendingAction || account.status !== 1" @click="resetSuperUserPassword(account)">重置临时密码</button></article></div>
      </section>

      <section v-else-if="adminSection === 'operations' && user?.is_super_admin" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>OPERATIONS</small><h1>系统运维</h1><p>平台容量概览；备份与恢复演练由 NAS 定时任务执行。</p></div></header>
        <div v-if="superOverview" class="ios-metric-grid"><article><b>{{ superOverview.groups }}</b><small>启用小组</small></article><article><b>{{ superOverview.users }}</b><small>启用账号</small></article><article><b>{{ superOverview.today_checkins }}</b><small>今日打卡</small></article><article><b>{{ (superOverview.asset_bytes / 1048576).toFixed(1) }} MB</b><small>资料存储</small></article></div>
        <article v-if="superOverview?.backup" class="ios-panel"><div class="panel-heading"><div><h2>最近备份</h2><small>{{ superOverview.backup.ok ? '备份成功' : '需要关注' }}</small></div><span class="ios-status" :class="{ active: superOverview.backup.ok }">{{ superOverview.backup.ok ? '正常' : '异常' }}</span></div><p>{{ superOverview.backup.finished_at || superOverview.backup.message || '尚无备份记录' }}</p></article>
        <article class="ios-panel"><div class="panel-heading"><div><h2>小组归档</h2><small>归档不会删除成员、任务、打卡或资料</small></div></div><div class="group-card-list"><div v-for="group in platformGroups" :key="group.id" class="operation-group-row"><div><b>{{ group.name }}</b><small>{{ group.code }} · {{ group.status === 0 ? '已归档' : '运行中' }}</small></div><button class="ios-secondary-button" type="button" :disabled="!!pendingAction" @click="setGroupStatus(group, group.status === 0 ? 1 : 0)">{{ group.status === 0 ? '恢复' : '归档' }}</button></div></div></article>
      </section>

      <section v-else-if="adminSection === 'approvals'" class="ios-admin-page">
        <header class="ios-page-heading heading-with-action"><div><small>REGISTRATION</small><h1>注册审批</h1><p>申请资料已经匹配 Excel 名单；确认无误后开通账号。</p></div><button class="ios-secondary-button" :disabled="!!pendingAction" type="button" @click="loadRegistrationRequests">{{ isPending('load-registrations') ? '刷新中…' : '刷新' }}</button></header>
        <div class="ios-member-toolbar"><span>待审批 {{ pendingRegistrationCount }} 人</span><span class="ios-info-chip">全部申请 {{ registrationRequests.length }}</span></div>
        <div v-if="registrationRequests.length" class="ios-approval-list">
          <article v-for="item in registrationRequests" :key="item.id" class="ios-approval-row">
            <span class="member-avatar-fallback">{{ (item.name || '?').slice(0,1) }}</span>
            <div class="member-copy"><b>{{ item.name }}</b><small>名单：{{ item.roster_name }} · 账号：@{{ item.username }}<template v-if="item.email"> · {{ item.email }}</template></small><small>提交于 {{ new Date(item.created_at).toLocaleString('zh-CN') }}</small></div>
            <span class="ios-status" :class="{ active: item.status === 'approved' }">{{ ({ pending: '待审批', approved: '已通过', rejected: '已拒绝' })[item.status] || item.status }}</span>
            <div v-if="item.status === 'pending'" class="member-row-actions"><button class="ios-text-danger" :disabled="!!pendingAction" type="button" @click="rejectRegistration(item)">拒绝</button><button :disabled="!!pendingAction" type="button" @click="approveRegistration(item)">{{ isPending(`approve-registration-${item.id}`) ? '处理中…' : '通过' }}</button></div>
            <small v-else-if="item.rejection_reason" class="approval-reason">原因：{{ item.rejection_reason }}</small>
          </article>
        </div>
        <div v-else class="ios-empty"><AppIcon name="check" :size="28" /><b>没有注册申请</b><span>新申请提交后会显示在这里。</span></div>
      </section>

      <section v-else-if="adminSection === 'library'" class="ios-admin-page">
        <header class="ios-page-heading heading-with-action"><div><small>LIBRARY</small><h1>学习资源</h1><p>决定哪些资料展示给成员；周任务挂载是独立操作，不影响资料库可见性。</p></div><button class="ios-secondary-button" :disabled="!!pendingAction" type="button" @click="syncNASResources">{{ isPending('sync-nas') ? '同步中…' : '同步 NAS' }}</button></header>
        <article class="ios-upload-panel"><span class="upload-illustration"><AppIcon name="upload" :size="28" /></span><div><b>上传新资源</b><small>上传到网站资源库；不会写入下方选中的 NAS 文件夹</small></div><select v-model="uploadCategory" :disabled="!canEditLearning || !!pendingAction" aria-label="上传资源类型"><option value="book">PDF 读物</option><option value="markdown">Markdown</option><option value="handout">讲义</option><option value="outline">提纲图片</option></select><input ref="uploadInput" :disabled="!canEditLearning || !!pendingAction" aria-label="选择要上传的资源文件" type="file" /><button :disabled="!canEditLearning || !!pendingAction" type="button" @click="uploadResource">{{ isPending('upload-resource') ? '上传中…' : '上传' }}</button></article>
        <div class="ios-library-tabs"><button v-for="section in resourceLibrary" :key="section.key || section.label" :class="{ active: activeLibrarySection?.key === section.key }" :aria-pressed="activeLibrarySection?.key === section.key" type="button" @click="chooseLibrarySection(section.key)"><span>{{ section.label }}</span><small>{{ section.count || 0 }}</small></button></div>
        <section v-if="activeLibrarySection" class="ios-panel ios-file-browser">
          <div class="file-browser-toolbar"><div><small>当前分类</small><h2>{{ activeLibrarySection.label }}</h2><small>先设置成员资料库可见性；需要时再挂载到 {{ resourceTargetLabel }}</small></div><div v-if="activeLibrarySection.managed_root && user?.is_super_admin" class="folder-create"><input v-model="newFolderName" :disabled="!!pendingAction" aria-label="新建 NAS 文件夹名称" placeholder="新文件夹名称" @keydown.enter.prevent="createResourceFolder" /><button class="ios-secondary-button" :disabled="!!pendingAction" type="button" @click="createResourceFolder">{{ isPending('create-folder') ? '创建中…' : '新建文件夹' }}</button></div></div>
          <div class="ios-form-grid"><label class="span-2"><span>搜索当前分类</span><input v-model.trim="resourceSearch" type="search" placeholder="输入标题、文件名或文件夹" /></label></div>
          <div v-if="activeLibrarySection.managed_root" class="ios-folder-strip"><button :class="{ active: !activeLibraryFolder }" type="button" @click="activeLibraryFolder = ''">全部文件</button><button v-for="folder in activeLibrarySection.folders || []" :key="folder.path" :class="{ active: activeLibraryFolder === folder.path }" type="button" @click="activeLibraryFolder = folder.path"><AppIcon name="library" :size="15" />{{ folder.path }}</button></div>
          <div v-if="activeLibraryFolder" class="folder-current"><span>当前文件夹：<b>{{ activeLibraryFolder }}</b></span><button v-if="user?.is_super_admin" class="ios-text-button" :disabled="!!pendingAction" type="button" @click="renameResourceFolder">{{ isPending('rename-folder') ? '重命名中…' : '重命名' }}</button></div>
          <div v-if="visibleLibraryItems.length" class="ios-resource-list ios-scroll-resource-list"><div v-for="item in visibleLibraryItems" :key="item.resource_key || item.id || item.url" class="ios-resource-row"><button class="resource-preview" type="button" @click="previewLibraryItem(item)"><span><AppIcon name="file" /></span><div><b>{{ item.title || item.original_name }}</b><small><template v-if="item.folder">{{ item.folder }} / </template>{{ item.original_name || '点击预览' }}</small><em :class="{ visible: item.visible_in_library }">{{ item.visible_in_library ? '成员资料库已显示' : '成员资料库未显示' }}</em></div></button><div class="ios-resource-actions"><button :class="item.visible_in_library ? 'ios-secondary-button' : 'ios-resource-use'" :disabled="!canManageResources || !!pendingAction" type="button" @click="toggleResourceVisibility(item)">{{ isPending(`resource-visibility-${item.resource_key}`) ? '保存中…' : (item.visible_in_library ? '移出资料库' : '显示到资料库') }}</button><button class="ios-text-button" :disabled="!canEditLearning || !!pendingAction" type="button" :aria-label="`将 ${item.title || item.original_name} 挂载到 ${resourceTargetLabel}`" @click="selectResourceForTask(item)">挂载到周任务</button></div></div></div><div v-else class="ios-empty compact">{{ resourceSearch ? '没有匹配的资源，请更换关键词' : '当前文件夹暂无可用资源' }}</div>
        </section>
      </section>

      <section v-else-if="adminSection === 'roster' && user?.is_super_admin" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>ROSTER</small><h1>报名名单</h1><p>Excel 批量同步与临时补录集中管理。</p></div></header>
        <div class="ios-panel-grid"><article class="ios-panel"><div class="panel-heading"><div><h2>Excel 名单同步</h2><small>读取分表和红色组长标记</small></div></div><div class="ios-stack"><input ref="rosterInput" :disabled="!!pendingAction" aria-label="选择报名名单 Excel" type="file" accept=".xlsx" /><button class="ios-secondary-button" :disabled="!!pendingAction" type="button" @click="previewRoster">{{ isPending('preview-roster') ? '解析中…' : '先预览' }}</button><div v-if="rosterPreview?.row_count" class="roster-preview-result"><b>{{ rosterPreview.row_count }}</b><span>个席位</span><b>{{ rosterPreview.leader_count }}</b><span>位组长</span><button :disabled="!!pendingAction" type="button" @click="importRoster">{{ isPending('import-roster') ? '同步中…' : '确认同步' }}</button></div></div></article><article class="ios-panel"><div class="panel-heading"><div><h2>临时补录</h2><small>适合新增成员或辅修关系</small></div></div><div class="ios-stack"><input v-model="rosterName" :disabled="!!pendingAction" aria-label="标准姓名" placeholder="标准姓名" /><select v-model="rosterGroupID" :disabled="!!pendingAction" aria-label="选择门训组"><option value="">选择门训组</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select><div class="ios-toggle-grid"><label><input v-model="rosterLeader" :disabled="!!pendingAction" type="checkbox" />组长</label><label><input v-model="rosterMinor" :disabled="!!pendingAction" type="checkbox" />辅修</label></div><button :disabled="!rosterName || !rosterGroupID || !!pendingAction" type="button" @click="addRosterEntry">{{ isPending('add-roster') ? '添加中…' : '添加名单席位' }}</button></div></article></div>
        <article class="ios-panel"><div class="panel-heading"><div><h2>当前名单</h2><small>显示 {{ filteredRosterEntries.length }} / {{ rosterEntries.length }} 个席位</small></div><button class="ios-text-button" :disabled="!!pendingAction" type="button" @click="loadRoster">{{ isPending('load-roster') ? '刷新中…' : '刷新' }}</button></div><div class="ios-stack"><label><span>搜索名单</span><input v-model.trim="rosterSearch" type="search" placeholder="姓名、小组、主辅修或注册状态" /></label></div><div v-if="filteredRosterEntries.length" class="ios-roster-list"><div v-for="entry in filteredRosterEntries" :key="entry.id"><span class="member-avatar-fallback small">{{ entry.name.slice(0,1) }}</span><div><b>{{ entry.name }}</b><small>{{ entry.group_name }} · {{ entry.is_minor ? '辅修' : '主修' }}</small></div><span v-if="entry.is_leader" class="ios-role 组长">组长</span><span v-else-if="entry.claimed_by_user_id" class="ios-status active">已注册</span><span v-else class="ios-status">待注册</span></div></div><div v-else class="ios-empty compact">没有匹配的名单席位</div></article>
      </section>

      <section v-else-if="adminSection === 'data'" class="ios-admin-page">
        <header class="ios-page-heading"><div><small>DATA</small><h1>数据工具</h1><p>导出报表和执行恢复操作。</p></div></header>
        <div class="ios-panel-grid"><article class="ios-panel"><div class="panel-heading"><div><h2>导出数据</h2><small>下载当前小组的数据文件</small></div></div><div class="ios-action-list"><button :disabled="!!pendingAction" type="button" @click="runExport('/admin/exports/checkins-detail','checkins-detail.csv','打卡明细已下载')"><AppIcon name="database" /><span><b>打卡明细</b><small>CSV 格式</small></span><AppIcon name="chevron" /></button><button :disabled="!!pendingAction" type="button" @click="runExport('/admin/exports/daily-summary','daily-summary.csv','每日汇总已下载')"><AppIcon name="chart" /><span><b>每日汇总</b><small>CSV 格式</small></span><AppIcon name="chevron" /></button><button :disabled="!!pendingAction" type="button" @click="runExport('/api/admin/exports/study-weeks','study-weeks.xlsx','任务计划已下载')"><AppIcon name="calendar" /><span><b>任务计划</b><small>Excel 格式</small></span><AppIcon name="chevron" /></button><button :disabled="!!pendingAction" type="button" @click="runExport('/admin/exports/local-backup','local-backup.json','备份已下载')"><AppIcon name="database" /><span><b>完整备份</b><small>JSON 格式</small></span><AppIcon name="chevron" /></button></div></article><article class="ios-panel"><div class="panel-heading"><div><h2>导入与恢复</h2><small>写入 {{ activeGroupName }}，请谨慎操作</small></div></div><div class="ios-stack"><label><span>导入任务计划 Excel</span><input ref="studyWeeksInput" :disabled="!canEditLearning || !!pendingAction" type="file" accept=".xlsx,.xlsm" /></label><button class="ios-secondary-button" :disabled="!canEditLearning || !!pendingAction" type="button" @click="runWeeksImport">{{ isPending('import-weeks') ? '导入中…' : '导入任务计划' }}</button><label><span>恢复新版完整备份 JSON</span><input ref="backupInput" :disabled="!canEditLearning || !!pendingAction" type="file" accept=".json" /></label><button class="ios-danger-button" :disabled="!canEditLearning || !!pendingAction" type="button" @click="runBackupImport">{{ isPending('import-backup') ? '恢复中…' : '恢复新版备份' }}</button></div></article></div>
        <article class="ios-panel ios-legacy-restore"><div class="panel-heading"><div><h2>从旧网站恢复</h2><small>直接读取旧版 config.json 和 records.json，恢复到 {{ activeGroupName }}</small></div></div><div class="ios-form-grid"><label><span>旧网站 config.json（必选）</span><input ref="legacyConfigInput" :disabled="!canEditLearning || !!pendingAction" type="file" accept=".json" @change="legacyPreview = null" /></label><label><span>旧网站打卡记录 records.json（可选）</span><input ref="legacyRecordsInput" :disabled="!canEditLearning || !!pendingAction" type="file" accept=".json" @change="legacyPreview = null" /></label></div><div class="legacy-actions"><button class="ios-secondary-button" :disabled="!canEditLearning || !!pendingAction" type="button" @click="previewLegacyRestore">{{ isPending('preview-legacy') ? '解析中…' : '解析并预览' }}</button><button v-if="legacyPreview" class="ios-danger-button" :disabled="!canEditLearning || !!pendingAction" type="button" @click="confirmLegacyRestore">{{ isPending('restore-legacy') ? '恢复中…' : '确认恢复到当前组' }}</button></div><div v-if="legacyPreview" class="legacy-preview"><div><b>{{ legacyPreview.weeks }}</b><span>周任务</span></div><div><b>{{ legacyPreview.sourceRecords }}</b><span>原始打卡记录</span></div><div><b>{{ legacyPreview.checkins }}</b><span>转换后打卡明细</span></div><p v-if="legacyPreview.unmatched.length"><strong>{{ legacyPreview.unmatched.length }} 个姓名未匹配，不会导入：</strong>{{ legacyPreview.unmatched.slice(0,12).join('、') }}<template v-if="legacyPreview.unmatched.length > 12"> 等</template></p><p v-else class="success-copy">所有打卡姓名均已匹配当前组成员。</p></div></article>
      </section>
    </main>
  </div>
</template>
