import { md5ArrayBuffer } from "./md5.js";

const CHUNK_SIZE = 5 * 1024 * 1024;

const state = {
  accessToken: "",
  refreshToken: "",
  profile: null,
  supportedTypes: [],
  accessibleDocuments: [],
  uploadedDocuments: [],
  searchResults: [],
  history: [],
  adminUsers: [],
  adminTags: [],
  adminFlatTags: [],
  adminConversations: [],
  refreshPromise: null,
  socket: null,
  socketPromise: null,
  activeCommandToken: "",
  activeAssistantBubble: null,
};

const nodes = {
  sessionPill: document.getElementById("session-pill"),
  profileSummary: document.getElementById("profile-summary"),
  logoutButton: document.getElementById("logout-button"),
  supportedTypes: document.getElementById("supported-types"),
  uploadFile: document.getElementById("upload-file"),
  chunkFile: document.getElementById("chunk-file"),
  uploadToolResult: document.getElementById("upload-tool-result"),
  chunkProgressFill: document.getElementById("chunk-progress-fill"),
  chunkProgressText: document.getElementById("chunk-progress-text"),
  accessibleDocuments: document.getElementById("accessible-documents"),
  uploadedDocuments: document.getElementById("uploaded-documents"),
  searchResults: document.getElementById("search-results"),
  chatStream: document.getElementById("chat-stream"),
  historyStream: document.getElementById("history-stream"),
  adminPanel: document.getElementById("admin-panel"),
  adminUsers: document.getElementById("admin-users"),
  adminTags: document.getElementById("admin-tags"),
  adminConversations: document.getElementById("admin-conversations"),
  consoleLog: document.getElementById("console-log"),
  previewDialog: document.getElementById("preview-dialog"),
  previewTitle: document.getElementById("preview-title"),
  previewContent: document.getElementById("preview-content"),
  connectChat: document.getElementById("connect-chat"),
  stopChat: document.getElementById("stop-chat"),
};

const forms = {
  login: document.getElementById("login-form"),
  register: document.getElementById("register-form"),
  upload: document.getElementById("upload-form"),
  chunkUpload: document.getElementById("chunk-upload-form"),
  fastUpload: document.getElementById("fast-upload-form"),
  uploadStatus: document.getElementById("upload-status-form"),
  search: document.getElementById("search-form"),
  chat: document.getElementById("chat-form"),
  assignOrgTags: document.getElementById("assign-org-tags-form"),
  createOrgTag: document.getElementById("create-org-tag-form"),
  deleteOrgTag: document.getElementById("delete-org-tag-form"),
  adminConversationFilter: document.getElementById("admin-conversation-filter-form"),
  adminDeleteDocument: document.getElementById("admin-delete-document-form"),
};

const STORAGE_KEYS = {
  accessToken: "paismart.accessToken",
  refreshToken: "paismart.refreshToken",
};

init();

function init() {
  bindEvents();
  loadStoredTokens();
  setChunkProgress(0, "等待分片上传。");
  renderAll();

  if (state.accessToken && state.refreshToken) {
    hydrateSession().catch((error) => {
      logLine(`恢复会话失败: ${error.message}`, "danger");
      clearSession();
    });
  } else {
    logLine("前端已加载，等待登录。");
  }
}

function bindEvents() {
  forms.login.addEventListener("submit", handleLogin);
  forms.register.addEventListener("submit", handleRegister);
  forms.upload.addEventListener("submit", handleSimpleUpload);
  forms.chunkUpload.addEventListener("submit", handleChunkUpload);
  forms.fastUpload.addEventListener("submit", handleFastUploadCheck);
  forms.uploadStatus.addEventListener("submit", handleUploadStatusCheck);
  forms.search.addEventListener("submit", handleSearch);
  forms.chat.addEventListener("submit", handleChatSubmit);
  forms.assignOrgTags.addEventListener("submit", handleAssignOrgTags);
  forms.createOrgTag.addEventListener("submit", handleCreateOrgTag);
  forms.deleteOrgTag.addEventListener("submit", handleDeleteOrgTag);
  forms.adminConversationFilter.addEventListener("submit", handleAdminConversationFilter);
  forms.adminDeleteDocument.addEventListener("submit", handleAdminDeleteDocument);

  nodes.logoutButton.addEventListener("click", handleLogout);
  nodes.connectChat.addEventListener("click", connectChatSocket);
  nodes.stopChat.addEventListener("click", stopChatStream);

  document.getElementById("reload-supported-types").addEventListener("click", loadSupportedTypes);
  document.getElementById("load-accessible-docs").addEventListener("click", loadAccessibleDocuments);
  document.getElementById("load-uploaded-docs").addEventListener("click", loadUploadedDocuments);
  document.getElementById("load-history").addEventListener("click", loadConversationHistory);
  document.getElementById("load-admin-users").addEventListener("click", loadAdminUsers);
  document.getElementById("load-admin-tags").addEventListener("click", loadAdminTags);
  document.getElementById("load-admin-conversations").addEventListener("click", loadAdminConversations);
  document.getElementById("clear-console").addEventListener("click", () => {
    nodes.consoleLog.innerHTML = "";
  });

  document.addEventListener("click", handleDelegatedActions);
}

async function handleLogin(event) {
  event.preventDefault();
  const formData = new FormData(forms.login);
  try {
    const data = await apiFetch("/api/v1/users/login", {
      method: "POST",
      body: {
        username: formData.get("username"),
        password: formData.get("password"),
      },
      skipRefresh: true,
    });
    setTokens(data.accessToken, data.refreshToken);
    await hydrateSession();
    logLine("登录成功。");
    forms.login.reset();
  } catch (error) {
    logLine(`登录失败: ${error.message}`, "danger");
  }
}

async function handleRegister(event) {
  event.preventDefault();
  const formData = new FormData(forms.register);
  const username = String(formData.get("username") || "");
  const password = String(formData.get("password") || "");

  try {
    await apiFetch("/api/v1/users/register", {
      method: "POST",
      body: { username, password },
      skipRefresh: true,
    });
    logLine(`用户 ${username} 注册成功，准备自动登录。`);
    forms.register.reset();

    const data = await apiFetch("/api/v1/users/login", {
      method: "POST",
      body: { username, password },
      skipRefresh: true,
    });
    setTokens(data.accessToken, data.refreshToken);
    await hydrateSession();
  } catch (error) {
    logLine(`注册失败: ${error.message}`, "danger");
  }
}

async function handleLogout() {
  try {
    if (state.accessToken) {
      await apiFetch("/api/v1/users/logout", { method: "POST" });
    }
  } catch (error) {
    logLine(`退出登录返回异常: ${error.message}`, "danger");
  } finally {
    clearSession();
    logLine("已清理本地会话。");
  }
}

async function handleSimpleUpload(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;

  const formData = new FormData(forms.upload);
  const file = formData.get("file");
  if (!(file instanceof File) || !file.size) {
    logLine("请选择要上传的文件。", "danger");
    return;
  }

  try {
    const payload = new FormData();
    payload.append("file", file);
    const orgTag = String(formData.get("orgTag") || "").trim();
    if (orgTag) {
      payload.append("orgTag", orgTag);
    }

    const result = await apiFetch("/api/v1/upload/simple", {
      method: "POST",
      body: payload,
    });
    fillMd5Tools(result.fileMd5);
    forms.upload.reset();
    nodes.uploadFile.setAttribute("accept", state.supportedTypes.join(","));
    showUploadToolResult(result);
    logLine(`上传成功: ${result.fileName} (${result.fileMd5})`);
    await Promise.all([loadUploadedDocuments(), loadAccessibleDocuments()]);
  } catch (error) {
    logLine(`上传失败: ${error.message}`, "danger");
  }
}

async function handleChunkUpload(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;

  const formData = new FormData(forms.chunkUpload);
  const file = formData.get("file");
  if (!(file instanceof File) || !file.size) {
    logLine("请选择要进行分片上传的文件。", "danger");
    return;
  }

  const orgTag = String(formData.get("orgTag") || "").trim();
  const isPublic = formData.get("isPublic") === "on";
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE);

  try {
    setChunkProgress(5, "正在计算文件 MD5...");
    const fileMd5 = md5ArrayBuffer(await file.arrayBuffer());
    fillMd5Tools(fileMd5);

    const checkResult = await apiFetch("/api/v1/upload/check", {
      method: "POST",
      body: { md5: fileMd5 },
    });

    if (checkResult.completed) {
      setChunkProgress(100, `文件已存在，无需重新上传。MD5: ${fileMd5}`);
      showUploadToolResult({ fileMd5, completed: true, uploadedChunks: [] });
      return;
    }

    const uploadedSet = new Set(checkResult.uploadedChunks || []);
    setChunkProgress(10, `MD5 计算完成，共 ${totalChunks} 个分片，待补传 ${totalChunks - uploadedSet.size} 个。`);

    let latestChunkResult = checkResult;
    for (let index = 0; index < totalChunks; index += 1) {
      if (uploadedSet.has(index)) {
        const resumedProgress = 10 + Math.round((uploadedSet.size / totalChunks) * 70);
        setChunkProgress(resumedProgress, `跳过已存在分片 ${index + 1}/${totalChunks}`);
        continue;
      }

      const chunk = file.slice(index * CHUNK_SIZE, Math.min(file.size, (index + 1) * CHUNK_SIZE));
      const payload = new FormData();
      payload.append("fileMd5", fileMd5);
      payload.append("fileName", file.name);
      payload.append("totalSize", String(file.size));
      payload.append("chunkIndex", String(index));
      payload.append("orgTag", orgTag);
      payload.append("isPublic", isPublic ? "true" : "false");
      payload.append("file", chunk, `${file.name}.part${index}`);

      latestChunkResult = await apiFetch("/api/v1/upload/chunk", {
        method: "POST",
        body: payload,
      });

      const chunkProgress = 10 + Math.round((Number(latestChunkResult.progress || 0) / 100) * 70);
      setChunkProgress(chunkProgress, `分片 ${index + 1}/${totalChunks} 上传完成，服务端进度 ${Number(latestChunkResult.progress || 0).toFixed(2)}%。`);
    }

    setChunkProgress(85, "分片上传完成，正在合并...");
    const mergeResult = await apiFetch("/api/v1/upload/merge", {
      method: "POST",
      body: { fileMd5, fileName: file.name },
    });

    setChunkProgress(100, `合并完成: ${mergeResult.objectUrl}`);
    showUploadToolResult({
      fileMd5,
      fileName: file.name,
      chunkSize: CHUNK_SIZE,
      totalChunks,
      chunkUpload: latestChunkResult,
      merge: mergeResult,
    });
    logLine(`分片上传完成: ${file.name} (${fileMd5})`);
    forms.chunkUpload.reset();
    await Promise.all([loadUploadedDocuments(), loadAccessibleDocuments()]);
  } catch (error) {
    setChunkProgress(0, `分片上传失败: ${error.message}`);
    logLine(`分片上传失败: ${error.message}`, "danger");
  }
}

async function handleFastUploadCheck(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;
  const formData = new FormData(forms.fastUpload);

  try {
    const result = await apiFetch("/api/v1/upload/fast-upload", {
      method: "POST",
      body: { md5: String(formData.get("md5") || "").trim() },
    });
    showUploadToolResult(result);
    logLine("秒传检查已完成。");
  } catch (error) {
    logLine(`秒传检查失败: ${error.message}`, "danger");
  }
}

async function handleUploadStatusCheck(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;
  const formData = new FormData(forms.uploadStatus);
  const fileMd5 = encodeURIComponent(String(formData.get("fileMd5") || "").trim());

  try {
    const result = await apiFetch(`/api/v1/upload/status?fileMd5=${fileMd5}`);
    showUploadToolResult(result);
    logLine("上传状态已刷新。");
  } catch (error) {
    logLine(`上传状态查询失败: ${error.message}`, "danger");
  }
}

async function handleSearch(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;

  const formData = new FormData(forms.search);
  const query = encodeURIComponent(String(formData.get("query") || "").trim());
  const topK = encodeURIComponent(String(formData.get("topK") || "6").trim());

  try {
    state.searchResults = await apiFetch(`/api/v1/search/hybrid?query=${query}&topK=${topK}`);
    renderSearchResults();
    logLine(`检索完成，返回 ${state.searchResults.length} 条结果。`);
  } catch (error) {
    logLine(`检索失败: ${error.message}`, "danger");
  }
}

async function handleChatSubmit(event) {
  event.preventDefault();
  if (!ensureAuthed()) return;

  const formData = new FormData(forms.chat);
  const question = String(formData.get("question") || "").trim();
  if (!question) {
    return;
  }

  try {
    await connectChatSocket();
    appendChatBubble("user", question);
    state.socket.send(JSON.stringify({ type: "message", content: question }));
    forms.chat.reset();
  } catch (error) {
    logLine(`发送聊天消息失败: ${error.message}`, "danger");
  }
}

async function hydrateSession() {
  await loadProfile();
  const tasks = [
    loadSupportedTypes(),
    loadAccessibleDocuments(),
    loadUploadedDocuments(),
    loadConversationHistory(),
  ];
  if (state.profile?.role === "ADMIN") {
    tasks.push(loadAdminUsers(), loadAdminTags(), loadAdminConversations());
  }
  await Promise.allSettled(tasks);
  renderAll();
}

async function loadProfile() {
  state.profile = await apiFetch("/api/v1/users/me");
  renderProfile();
}

async function loadSupportedTypes() {
  if (!ensureAuthed(false)) return;
  try {
    state.supportedTypes = await apiFetch("/api/v1/upload/supported-types");
    renderSupportedTypes();
    logLine("支持的上传类型已更新。");
  } catch (error) {
    logLine(`加载支持类型失败: ${error.message}`, "danger");
  }
}

async function loadAccessibleDocuments() {
  if (!ensureAuthed(false)) return;
  try {
    state.accessibleDocuments = await apiFetch("/api/v1/documents/accessible");
    renderDocumentLists();
  } catch (error) {
    logLine(`加载可访问文档失败: ${error.message}`, "danger");
  }
}

async function loadUploadedDocuments() {
  if (!ensureAuthed(false)) return;
  try {
    state.uploadedDocuments = await apiFetch("/api/v1/documents/uploads");
    renderDocumentLists();
  } catch (error) {
    logLine(`加载上传文档失败: ${error.message}`, "danger");
  }
}

async function loadConversationHistory() {
  if (!ensureAuthed(false)) return;
  try {
    state.history = await apiFetch("/api/v1/users/conversation");
    renderHistory();
  } catch (error) {
    logLine(`加载会话历史失败: ${error.message}`, "danger");
  }
}

async function loadAdminUsers() {
  if (!ensureAdmin()) return;
  try {
    const result = await apiFetch("/api/v1/admin/users/list?page=1&size=20");
    state.adminUsers = result.content || [];
    renderAdminUsers();
    logLine("管理员用户列表已更新。");
  } catch (error) {
    logLine(`加载管理员用户列表失败: ${error.message}`, "danger");
  }
}

async function loadAdminTags() {
  if (!ensureAdmin()) return;
  try {
    state.adminTags = await apiFetch("/api/v1/admin/org-tags/tree");
    state.adminFlatTags = flattenTagTree(state.adminTags);
    renderAdminTags();
    logLine("组织树已更新。");
  } catch (error) {
    logLine(`加载组织树失败: ${error.message}`, "danger");
  }
}

async function loadAdminConversations() {
  if (!ensureAdmin()) return;
  try {
    state.adminConversations = await apiFetch("/api/v1/admin/conversation");
    renderAdminConversations();
    logLine("管理员会话审计已更新。");
  } catch (error) {
    logLine(`加载会话审计失败: ${error.message}`, "danger");
  }
}

async function handleAssignOrgTags(event) {
  event.preventDefault();
  if (!ensureAdmin()) return;

  const formData = new FormData(forms.assignOrgTags);
  const userId = Number(formData.get("userId"));
  const orgTags = String(formData.get("orgTags") || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

  try {
    await apiFetch(`/api/v1/admin/users/${userId}/org-tags`, {
      method: "PUT",
      body: { orgTags },
    });
    logLine(`已更新用户 ${userId} 的组织标签。`);
    await loadAdminUsers();
    forms.assignOrgTags.reset();
  } catch (error) {
    logLine(`分配组织标签失败: ${error.message}`, "danger");
  }
}

async function handleCreateOrgTag(event) {
  event.preventDefault();
  if (!ensureAdmin()) return;

  const formData = new FormData(forms.createOrgTag);
  const parentTag = String(formData.get("parentTag") || "").trim();

  try {
    await apiFetch("/api/v1/admin/org-tags", {
      method: "POST",
      body: {
        tagId: String(formData.get("tagId") || "").trim(),
        name: String(formData.get("name") || "").trim(),
        description: String(formData.get("description") || "").trim(),
        parentTag: parentTag || null,
      },
    });
    logLine("组织标签已创建。");
    await loadAdminTags();
    forms.createOrgTag.reset();
  } catch (error) {
    logLine(`创建组织标签失败: ${error.message}`, "danger");
  }
}

async function handleDeleteOrgTag(event) {
  event.preventDefault();
  if (!ensureAdmin()) return;

  const formData = new FormData(forms.deleteOrgTag);
  const tagId = String(formData.get("tagId") || "").trim();
  const strategy = String(formData.get("strategy") || "protect").trim();

  if (!tagId) {
    logLine("删除组织标签前需要填写 tagId。", "danger");
    return;
  }

  try {
    await apiFetch(`/api/v1/admin/org-tags/${encodeURIComponent(tagId)}?strategy=${encodeURIComponent(strategy)}`, {
      method: "DELETE",
    });
    logLine(`组织标签已删除: ${tagId}`);
    await loadAdminTags();
    forms.deleteOrgTag.reset();
  } catch (error) {
    logLine(`删除组织标签失败: ${error.message}`, "danger");
  }
}

async function handleAdminConversationFilter(event) {
  event.preventDefault();
  if (!ensureAdmin()) return;

  const formData = new FormData(forms.adminConversationFilter);
  const params = new URLSearchParams();
  const userId = String(formData.get("userId") || "").trim();
  const startDate = String(formData.get("startDate") || "").trim();
  const endDate = String(formData.get("endDate") || "").trim();

  if (userId) params.set("userid", userId);
  if (startDate) params.set("start_date", startDate);
  if (endDate) params.set("end_date", endDate);

  try {
    state.adminConversations = await apiFetch(`/api/v1/admin/conversation?${params.toString()}`);
    renderAdminConversations();
    logLine("管理员会话过滤结果已更新。");
  } catch (error) {
    logLine(`查询管理员会话失败: ${error.message}`, "danger");
  }
}

async function handleAdminDeleteDocument(event) {
  event.preventDefault();
  if (!ensureAdmin()) return;

  const formData = new FormData(forms.adminDeleteDocument);
  const userId = String(formData.get("userId") || "").trim();
  const fileMd5 = String(formData.get("fileMd5") || "").trim();
  if (!userId || !fileMd5) {
    logLine("管理员删除文档需要同时填写 userId 和 fileMd5。", "danger");
    return;
  }

  const confirmed = window.confirm(`确认删除用户 ${userId} 的文件 ${fileMd5}？`);
  if (!confirmed) {
    return;
  }

  try {
    await apiFetch(`/api/v1/documents/${encodeURIComponent(fileMd5)}?userId=${encodeURIComponent(userId)}`, {
      method: "DELETE",
    });
    logLine(`管理员已删除用户 ${userId} 的文件 ${fileMd5}。`);
    forms.adminDeleteDocument.reset();
    await Promise.all([loadAccessibleDocuments(), loadUploadedDocuments()]);
  } catch (error) {
    logLine(`管理员删除文档失败: ${error.message}`, "danger");
  }
}

async function connectChatSocket() {
  if (!ensureAuthed()) {
    throw new Error("请先登录");
  }

  if (state.socket && state.socket.readyState === WebSocket.OPEN) {
    return state.socket;
  }

  if (state.socketPromise) {
    return state.socketPromise;
  }

  state.socketPromise = (async () => {
    const tokenData = await apiFetch("/api/v1/chat/websocket-token");
    const wsUrl = buildWebSocketUrl(tokenData.cmdToken);

    await new Promise((resolve, reject) => {
      closeSocket();
      const socket = new WebSocket(wsUrl);
      state.socket = socket;

      socket.addEventListener("open", () => {
        logLine("WebSocket 已连接。");
        resolve();
      });

      socket.addEventListener("message", (event) => {
        handleChatMessage(event.data);
      });

      socket.addEventListener("close", () => {
        logLine("WebSocket 已关闭。");
        if (state.activeCommandToken) {
          appendChatBubble("system", "连接已关闭，本次生成已中断。");
        }
        state.activeCommandToken = "";
        state.activeAssistantBubble = null;
        state.socket = null;
      });

      socket.addEventListener("error", () => {
        reject(new Error("WebSocket 连接失败"));
      });
    });

    return state.socket;
  })();

  try {
    return await state.socketPromise;
  } finally {
    state.socketPromise = null;
  }
}

function handleChatMessage(rawData) {
  let message;
  try {
    message = JSON.parse(rawData);
  } catch (error) {
    appendChatBubble("system", `收到无法解析的消息: ${rawData}`);
    return;
  }

  if (message.type === "started") {
    state.activeCommandToken = message._internal_cmd_token || "";
    state.activeAssistantBubble = appendChatBubble("assistant", "");
    logLine(`聊天流已开始: ${state.activeCommandToken}`);
    return;
  }

  if (typeof message.chunk === "string") {
    if (!state.activeAssistantBubble) {
      state.activeAssistantBubble = appendChatBubble("assistant", "");
    }
    const contentNode = state.activeAssistantBubble.querySelector("[data-role-content]");
    contentNode.textContent += message.chunk;
    scrollToBottom(nodes.chatStream);
    return;
  }

  if (message.type === "completion") {
    logLine(`聊天流已结束: ${message.status || "finished"}`);
    state.activeCommandToken = "";
    state.activeAssistantBubble = null;
    loadConversationHistory();
    return;
  }

  if (message.error) {
    appendChatBubble("system", message.error);
    state.activeCommandToken = "";
    state.activeAssistantBubble = null;
  }
}

function stopChatStream() {
  if (!state.socket || state.socket.readyState !== WebSocket.OPEN || !state.activeCommandToken) {
    logLine("当前没有可停止的生成。");
    return;
  }

  state.socket.send(JSON.stringify({ type: "stop", _internal_cmd_token: state.activeCommandToken }));
  logLine("已发送停止命令。");
}

async function handleDelegatedActions(event) {
  const target = event.target.closest("[data-action]");
  if (!target) return;

  const { action, md5 = "", filename = "", userId = "" } = target.dataset;
  if (!ensureAuthed()) return;

  switch (action) {
    case "preview-doc":
      await previewDocument(md5, filename);
      break;
    case "download-doc":
      await downloadDocument(md5, filename);
      break;
    case "delete-doc":
      await deleteDocument(md5, userId);
      break;
    case "use-search-in-chat":
      forms.chat.elements.question.value = target.dataset.prompt || "";
      document.getElementById("chat-panel").scrollIntoView({ behavior: "smooth", block: "start" });
      break;
    case "fill-assign-user":
      forms.assignOrgTags.elements.userId.value = target.dataset.userId || "";
      forms.assignOrgTags.scrollIntoView({ behavior: "smooth", block: "center" });
      break;
    case "fill-parent-tag":
      forms.createOrgTag.elements.parentTag.value = target.dataset.tagId || "";
      forms.createOrgTag.scrollIntoView({ behavior: "smooth", block: "center" });
      break;
    case "fill-delete-tag":
      forms.deleteOrgTag.elements.tagId.value = target.dataset.tagId || "";
      forms.deleteOrgTag.scrollIntoView({ behavior: "smooth", block: "center" });
      break;
    default:
      break;
  }
}

async function previewDocument(fileMd5, fileName) {
  try {
    const params = new URLSearchParams();
    if (fileMd5) params.set("fileMd5", fileMd5);
    if (!fileMd5 && fileName) params.set("fileName", fileName);
    const result = await apiFetch(`/api/v1/documents/preview?${params.toString()}`);
    nodes.previewTitle.textContent = `${result.fileName} 预览`;
    nodes.previewContent.textContent = result.content || "";
    nodes.previewDialog.showModal();
  } catch (error) {
    logLine(`预览失败: ${error.message}`, "danger");
  }
}

async function downloadDocument(fileMd5, fileName) {
  try {
    const params = new URLSearchParams();
    if (fileMd5) params.set("fileMd5", fileMd5);
    if (!fileMd5 && fileName) params.set("fileName", fileName);
    const result = await apiFetch(`/api/v1/documents/download?${params.toString()}`);
    window.open(result.downloadUrl, "_blank", "noopener");
  } catch (error) {
    logLine(`下载链接生成失败: ${error.message}`, "danger");
  }
}

async function deleteDocument(fileMd5, userId) {
  const isAdminDelete = state.profile?.role === "ADMIN" && userId && Number(userId) !== state.profile.id;
  const confirmed = window.confirm(
    isAdminDelete ? "确认删除该用户的文档记录？" : "确认删除该文档？删除会清理索引、向量和对象存储。"
  );
  if (!confirmed) {
    return;
  }

  try {
    const suffix = isAdminDelete ? `?userId=${encodeURIComponent(userId)}` : "";
    await apiFetch(`/api/v1/documents/${encodeURIComponent(fileMd5)}${suffix}`, { method: "DELETE" });
    logLine(`文档已删除: ${fileMd5}`);
    await Promise.all([loadAccessibleDocuments(), loadUploadedDocuments()]);
  } catch (error) {
    logLine(`删除失败: ${error.message}`, "danger");
  }
}

function renderAll() {
  renderProfile();
  renderSupportedTypes();
  renderDocumentLists();
  renderSearchResults();
  renderHistory();
  renderAdminUsers();
  renderAdminTags();
  renderAdminConversations();
}

function renderProfile() {
  const authed = Boolean(state.profile);
  nodes.sessionPill.textContent = authed ? `已登录 · ${state.profile.username}` : "未登录";
  nodes.logoutButton.disabled = !authed;
  nodes.connectChat.disabled = !authed;
  nodes.stopChat.disabled = !authed;

  if (!authed) {
    nodes.profileSummary.innerHTML = "当前没有有效会话。登录后会自动拉取个人信息、支持的上传类型和文档概览。";
    nodes.adminPanel.classList.add("hidden");
    return;
  }

  nodes.profileSummary.innerHTML = `
    <strong>${escapeHtml(state.profile.username)}</strong>
    <span>角色：${escapeHtml(state.profile.role)}</span>
    <span>主组织：${escapeHtml(state.profile.primaryOrg || "未设置")}</span>
    <span>拥有标签：${(state.profile.orgTags || []).map(escapeHtml).join(" / ") || "无"}</span>
    <span>可访问文档：${state.accessibleDocuments.length}</span>
    <span>我的上传：${state.uploadedDocuments.length}</span>
  `;

  if (state.profile.role === "ADMIN") {
    nodes.adminPanel.classList.remove("hidden");
  } else {
    nodes.adminPanel.classList.add("hidden");
  }
}

function renderSupportedTypes() {
  if (!state.supportedTypes.length) {
    nodes.supportedTypes.innerHTML = `<span class="empty-state">登录后加载允许上传的扩展名。</span>`;
    nodes.uploadFile.removeAttribute("accept");
    nodes.chunkFile.removeAttribute("accept");
    return;
  }

  nodes.supportedTypes.innerHTML = state.supportedTypes
    .map((type) => `<span class="chip">${escapeHtml(type)}</span>`)
    .join("");
  nodes.uploadFile.setAttribute("accept", state.supportedTypes.join(","));
  nodes.chunkFile.setAttribute("accept", state.supportedTypes.join(","));
}

function renderDocumentLists() {
  renderDocumentCollection(nodes.accessibleDocuments, state.accessibleDocuments, false);
  renderDocumentCollection(nodes.uploadedDocuments, state.uploadedDocuments, true);
  renderProfile();
}

function renderDocumentCollection(container, documents, ownerList) {
  if (!documents.length) {
    container.innerHTML = `<div class="empty-state">暂无文档。</div>`;
    return;
  }

  container.innerHTML = documents
    .map((doc) => {
      const fileMd5 = getDocumentMd5(doc);
      const fileName = doc.fileName || fileMd5;
      const canDelete = ownerList || canDeleteDocument(doc);
      const processingStatus = normalizeProcessingStatus(doc.processingStatus);
      return `
        <article class="document-card">
          <h4>${escapeHtml(fileName)}</h4>
          <div class="meta">
            <div>MD5: ${escapeHtml(fileMd5)}</div>
            <div>归属用户: ${escapeHtml(String(doc.userId ?? "-"))}</div>
            <div>组织标签: ${escapeHtml(doc.orgTagName || doc.orgTag || "-")}</div>
            <div>大小: ${formatBytes(doc.totalSize || 0)}</div>
            <div>公开: ${doc.isPublic ? "是" : "否"}</div>
            <div>上传状态: ${escapeHtml(getUploadStatusLabel(doc.status))}</div>
            <div>资料库状态: <span class="processing-chip processing-${escapeHtml(processingStatus)}">${escapeHtml(getProcessingStatusLabel(processingStatus))}</span></div>
          </div>
          <div class="document-actions">
            <button class="secondary-button" data-action="preview-doc" data-md5="${escapeHtml(fileMd5)}" data-filename="${escapeHtml(doc.fileName || "")}">预览</button>
            <button class="secondary-button" data-action="download-doc" data-md5="${escapeHtml(fileMd5)}" data-filename="${escapeHtml(doc.fileName || "")}">下载</button>
            ${canDelete ? `<button class="secondary-button" data-action="delete-doc" data-md5="${escapeHtml(fileMd5)}" data-user-id="${escapeHtml(String(doc.userId ?? ""))}">删除</button>` : ""}
          </div>
          <time>${formatTime(doc.createdAt)}</time>
        </article>
      `;
    })
    .join("");
}

function renderSearchResults() {
  if (!state.searchResults.length) {
    nodes.searchResults.innerHTML = `<div class="empty-state">还没有检索结果。</div>`;
    return;
  }

  nodes.searchResults.innerHTML = state.searchResults
    .map((item) => {
      const prompt = `请基于文件《${item.fileName || item.fileMd5}》第 ${item.chunkId} 个片段继续解释：\n${item.textContent}`;
      return `
        <article class="search-card">
          <h4>${escapeHtml(item.fileName || item.fileMd5)}</h4>
          <div class="meta">
            <div>Score: ${Number(item.score || 0).toFixed(4)}</div>
            <div>Chunk: ${escapeHtml(String(item.chunkId))}</div>
            <div>OrgTag: ${escapeHtml(item.orgTag || "-")}</div>
          </div>
          <p>${escapeHtml(item.textContent || "")}</p>
          <div class="button-row">
            <button class="secondary-button" data-action="preview-doc" data-md5="${escapeHtml(item.fileMd5)}" data-filename="${escapeHtml(item.fileName || "")}">预览原文</button>
            <button class="secondary-button" data-action="download-doc" data-md5="${escapeHtml(item.fileMd5)}" data-filename="${escapeHtml(item.fileName || "")}">下载</button>
            <button class="secondary-button" data-action="use-search-in-chat" data-prompt="${escapeHtml(prompt)}">送入聊天</button>
          </div>
        </article>
      `;
    })
    .join("");
}

function renderHistory() {
  if (!state.history.length) {
    nodes.historyStream.innerHTML = `<div class="empty-state">当前没有会话历史。</div>`;
    return;
  }

  nodes.historyStream.innerHTML = state.history
    .map(
      (item) => `
        <article class="history-card">
          <div class="message-role">${escapeHtml(item.role || "message")}</div>
          <div>${escapeHtml(item.content || "")}</div>
          <time>${formatTime(item.createdAt)}</time>
        </article>
      `
    )
    .join("");
}

function renderAdminUsers() {
  if (!state.adminUsers.length) {
    nodes.adminUsers.innerHTML = `<div class="empty-state">点击上方“用户”载入管理员数据。</div>`;
    return;
  }

  nodes.adminUsers.innerHTML = state.adminUsers
    .map(
      (user) => `
        <article class="admin-card">
          <h4>${escapeHtml(user.username)}</h4>
          <div class="meta">
            <div>UserID: ${escapeHtml(String(user.userId))}</div>
            <div>Role: ${escapeHtml(user.role)}</div>
            <div>PrimaryOrg: ${escapeHtml(user.primaryOrg || "-")}</div>
            <div>Tags: ${(user.orgTags || []).map((tag) => escapeHtml(tag.name || tag.tagId)).join(" / ") || "无"}</div>
          </div>
          <div class="button-row">
            <button class="secondary-button" data-action="fill-assign-user" data-user-id="${escapeHtml(String(user.userId))}">带入分配表单</button>
          </div>
        </article>
      `
    )
    .join("");
}

function renderAdminTags() {
  if (!state.adminTags.length) {
    nodes.adminTags.innerHTML = `<div class="empty-state">点击上方“组织标签”查看树结构。</div>`;
    return;
  }
  nodes.adminTags.innerHTML = renderTagTree(state.adminTags);
}

function renderAdminConversations() {
  if (!state.adminConversations.length) {
    nodes.adminConversations.innerHTML = `<div class="empty-state">点击上方“会话审计”加载数据。</div>`;
    return;
  }

  nodes.adminConversations.innerHTML = state.adminConversations
    .map(
      (item) => `
        <article class="admin-card">
          <h4>${escapeHtml(item.username)} · ${escapeHtml(item.role)}</h4>
          <div>${escapeHtml(item.content || "")}</div>
          <time>${formatTime(item.createdAt)} · Conversation ${escapeHtml(item.conversationId)}</time>
        </article>
      `
    )
    .join("");
}

function appendChatBubble(role, content) {
  const bubble = document.createElement("article");
  bubble.className = `message-bubble ${role}`;
  bubble.innerHTML = `
    <div class="message-role">${escapeHtml(role)}</div>
    <div data-role-content>${escapeHtml(content)}</div>
  `;
  nodes.chatStream.appendChild(bubble);
  scrollToBottom(nodes.chatStream);
  return bubble;
}

function renderTagTree(nodesList, depth = 0) {
  return nodesList
    .map((tag) => {
      const indent = depth * 18;
      const children = Array.isArray(tag.children) ? renderTagTree(tag.children, depth + 1) : "";
      return `
        <article class="admin-card" style="margin-left:${indent}px">
          <h4>${escapeHtml(tag.name || tag.tagId)}</h4>
          <div class="meta">
            <div>TagID: ${escapeHtml(tag.tagId)}</div>
            <div>Parent: ${escapeHtml(tag.parentTag || "-")}</div>
            <div>${escapeHtml(tag.description || "无描述")}</div>
          </div>
          <div class="button-row">
            <button class="secondary-button" data-action="fill-parent-tag" data-tag-id="${escapeHtml(tag.tagId)}">带入父标签</button>
            <button class="secondary-button" data-action="fill-delete-tag" data-tag-id="${escapeHtml(tag.tagId)}">带入删除表单</button>
          </div>
        </article>
        ${children}
      `;
    })
    .join("");
}

async function apiFetch(path, options = {}) {
  const { method = "GET", body, headers = {}, skipRefresh = false, retry = true } = options;
  const requestHeaders = new Headers(headers);
  const init = { method, headers: requestHeaders };

  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    requestHeaders.set("Content-Type", "application/json");
    init.body = JSON.stringify(body);
  }

  if (state.accessToken) {
    requestHeaders.set("Authorization", `Bearer ${state.accessToken}`);
  }

  let response = await fetch(path, init);

  if (response.status === 401 && !skipRefresh && retry && state.refreshToken) {
    const refreshed = await refreshSession();
    if (refreshed) {
      return apiFetch(path, { ...options, retry: false });
    }
  }

  const payload = await readJson(response);
  if (!response.ok) {
    throw new Error(payload?.message || payload?.error || `${response.status} ${response.statusText}`);
  }
  return payload?.data ?? payload;
}

async function refreshSession() {
  if (!state.refreshToken) {
    return false;
  }
  if (state.refreshPromise) {
    return state.refreshPromise;
  }

  state.refreshPromise = (async () => {
    try {
      const response = await fetch("/api/v1/auth/refreshToken", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refreshToken: state.refreshToken }),
      });
      const payload = await readJson(response);
      if (!response.ok) {
        throw new Error(payload?.message || "refresh failed");
      }
      const data = payload?.data ?? payload;
      setTokens(data.accessToken, data.refreshToken);
      logLine("访问令牌已自动刷新。");
      return true;
    } catch (error) {
      logLine(`刷新令牌失败: ${error.message}`, "danger");
      clearSession();
      return false;
    } finally {
      state.refreshPromise = null;
    }
  })();

  return state.refreshPromise;
}

function setTokens(accessToken, refreshToken) {
  state.accessToken = accessToken;
  state.refreshToken = refreshToken;
  window.localStorage.setItem(STORAGE_KEYS.accessToken, accessToken);
  window.localStorage.setItem(STORAGE_KEYS.refreshToken, refreshToken);
}

function loadStoredTokens() {
  state.accessToken = window.localStorage.getItem(STORAGE_KEYS.accessToken) || "";
  state.refreshToken = window.localStorage.getItem(STORAGE_KEYS.refreshToken) || "";
}

function clearSession() {
  closeSocket();
  state.accessToken = "";
  state.refreshToken = "";
  state.profile = null;
  state.supportedTypes = [];
  state.accessibleDocuments = [];
  state.uploadedDocuments = [];
  state.searchResults = [];
  state.history = [];
  state.adminUsers = [];
  state.adminTags = [];
  state.adminFlatTags = [];
  state.adminConversations = [];
  state.activeCommandToken = "";
  state.activeAssistantBubble = null;
  window.localStorage.removeItem(STORAGE_KEYS.accessToken);
  window.localStorage.removeItem(STORAGE_KEYS.refreshToken);
  setChunkProgress(0, "等待分片上传。");
  renderAll();
}

function closeSocket() {
  if (state.socket) {
    try {
      state.socket.close();
    } catch (_) {
      // ignore
    }
  }
  state.socket = null;
  state.socketPromise = null;
  state.activeCommandToken = "";
  state.activeAssistantBubble = null;
}

function showUploadToolResult(payload) {
  nodes.uploadToolResult.textContent = JSON.stringify(payload, null, 2);
}

function fillMd5Tools(fileMd5) {
  forms.fastUpload.elements.md5.value = fileMd5;
  forms.uploadStatus.elements.fileMd5.value = fileMd5;
}

function getDocumentMd5(doc) {
  return doc?.fileMd5 || doc?.fileMD5 || "";
}

function normalizeProcessingStatus(status) {
  return String(status || "pending").trim().toLowerCase() || "pending";
}

function getProcessingStatusLabel(status) {
  switch (normalizeProcessingStatus(status)) {
    case "processing":
      return "处理中";
    case "indexed":
      return "已入资料库";
    case "empty":
      return "无可检索文本";
    case "failed":
      return "处理失败";
    case "pending":
    default:
      return "待处理";
  }
}

function getUploadStatusLabel(status) {
  switch (Number(status)) {
    case 1:
      return "上传完成";
    case 2:
      return "上传失败";
    case 0:
    default:
      return "上传中";
  }
}

function setChunkProgress(percent, message) {
  const normalized = Math.max(0, Math.min(100, Number(percent || 0)));
  nodes.chunkProgressFill.style.width = `${normalized}%`;
  nodes.chunkProgressText.textContent = message;
}

function buildWebSocketUrl(cmdToken) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/chat/${cmdToken}`;
}

function flattenTagTree(nodesList, result = []) {
  for (const tag of nodesList || []) {
    result.push(tag);
    if (Array.isArray(tag.children) && tag.children.length > 0) {
      flattenTagTree(tag.children, result);
    }
  }
  return result;
}

function ensureAuthed(logIfMissing = true) {
  const ok = Boolean(state.accessToken && state.refreshToken);
  if (!ok && logIfMissing) {
    logLine("请先登录。", "danger");
  }
  return ok;
}

function ensureAdmin() {
  if (!ensureAuthed()) return false;
  if (state.profile?.role !== "ADMIN") {
    logLine("当前账号不是管理员。", "danger");
    return false;
  }
  return true;
}

function canDeleteDocument(doc) {
  if (!state.profile) return false;
  return state.profile.role === "ADMIN" || Number(doc.userId) === Number(state.profile.id);
}

function logLine(message, tone = "normal") {
  const entry = document.createElement("div");
  entry.className = `console-entry ${tone === "danger" ? "danger-text" : ""}`;
  entry.innerHTML = `<div class="console-time">${new Date().toLocaleTimeString("zh-CN", { hour12: false })}</div><div>${escapeHtml(message)}</div>`;
  nodes.consoleLog.prepend(entry);
}

function readJson(response) {
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("application/json")) {
    return Promise.resolve(null);
  }
  return response.json().catch(() => null);
}

function formatBytes(value) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 ** 3) return `${(size / 1024 ** 2).toFixed(1)} MB`;
  return `${(size / 1024 ** 3).toFixed(1)} GB`;
}

function formatTime(value) {
  if (!value) return "未知时间";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return escapeHtml(String(value));
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function scrollToBottom(container) {
  container.scrollTop = container.scrollHeight;
}
