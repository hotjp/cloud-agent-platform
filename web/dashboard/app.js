/**
 * Cloud Agent Platform Dashboard
 * Real-time task monitoring via WebSocket
 */

// API Configuration - use current host, no hardcoding
const API_BASE = window.location.origin;
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_PROTOCOL}//${window.location.host}/api/v1/ws`;

// State
let tasks = [];
let selectedTaskId = null;
let ws = null;
let wsConnected = false;
let wsReconnectTimeout = null;
let wsReconnectDelay = 3000;

// DOM Elements
const taskListEl = document.getElementById('taskList');
const detailPanelEl = document.getElementById('detailPanel');
const statusFilterEl = document.getElementById('statusFilter');
const refreshBtnEl = document.getElementById('refreshBtn');
const connectionStatusEl = document.getElementById('connectionStatus');

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    loadTasks();
    initWebSocket();
    setupEventListeners();
});

function setupEventListeners() {
    if (refreshBtnEl) {
        refreshBtnEl.addEventListener('click', () => {
            loadTasks();
        });
    }

    if (statusFilterEl) {
        statusFilterEl.addEventListener('change', () => {
            renderTaskList();
        });
    }
}

// WebSocket Connection
function initWebSocket() {
    updateConnectionStatus('connecting');

    try {
        ws = new WebSocket(WS_URL);

        ws.onopen = () => {
            console.log('WebSocket connected');
            wsConnected = true;
            wsReconnectDelay = 3000;
            updateConnectionStatus('connected');
        };

        ws.onmessage = (event) => {
            try {
                const messages = event.data.split('\n');
                for (const msgStr of messages) {
                    if (!msgStr.trim()) continue;
                    const msg = JSON.parse(msgStr);
                    handleWSEvent(msg);
                }
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };

        ws.onclose = () => {
            console.log('WebSocket disconnected');
            wsConnected = false;
            updateConnectionStatus('disconnected');
            scheduleReconnect();
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            wsConnected = false;
            updateConnectionStatus('disconnected');
        };
    } catch (err) {
        console.error('Failed to create WebSocket:', err);
        updateConnectionStatus('disconnected');
        scheduleReconnect();
    }
}

function scheduleReconnect() {
    if (wsReconnectTimeout) {
        clearTimeout(wsReconnectTimeout);
    }
    wsReconnectTimeout = setTimeout(() => {
        console.log('Attempting to reconnect...');
        updateConnectionStatus('connecting');
        initWebSocket();
    }, wsReconnectDelay);
    // Exponential backoff, max 30s
    wsReconnectDelay = Math.min(wsReconnectDelay * 1.5, 30000);
}

function handleWSEvent(event) {
    switch (event.type) {
        case 'task.status_changed':
            handleTaskStatusChanged(event);
            break;
        case 'task.created':
            loadTasks();
            break;
        case 'task.approval_required':
            showToast('Approval required for task', 'warning');
            break;
        case 'agent.thought':
        case 'artifact.created':
            if (selectedTaskId && selectedTaskId === event.taskId) {
                renderTaskDetail();
            }
            break;
        default:
            console.log('WS Event:', event.type);
    }
}

function handleTaskStatusChanged(event) {
    const taskId = event.taskId;
    let newStatus = event.status;

    if (event.payload) {
        try {
            const payload = typeof event.payload === 'string' ? JSON.parse(event.payload) : event.payload;
            newStatus = payload.new || payload.status || newStatus;
        } catch (e) {
            // use default
        }
    }

    const task = tasks.find(t => t.task_id === taskId || t.taskId === taskId);
    if (task) {
        task.status = newStatus;
        renderTaskList();

        if (selectedTaskId === taskId) {
            renderTaskDetail();
        }
    }
}

function updateConnectionStatus(status) {
    if (!connectionStatusEl) return;
    const dot = connectionStatusEl.querySelector('.status-dot');
    const text = connectionStatusEl.querySelector('.status-text');

    if (dot) dot.className = 'status-dot ' + status;
    if (text) {
        switch (status) {
            case 'connected': text.textContent = 'Connected'; break;
            case 'connecting': text.textContent = 'Connecting...'; break;
            case 'disconnected': text.textContent = 'Disconnected'; break;
        }
    }
}

// API Calls
async function loadTasks() {
    if (!taskListEl) return;

    try {
        if (refreshBtnEl) refreshBtnEl.classList.add('spinning');

        const params = new URLSearchParams();
        params.append('page', '1');
        params.append('pageSize', '100');

        const status = statusFilterEl ? statusFilterEl.value : '';
        if (status) {
            params.append('status', status);
        }

        const response = await fetch(`${API_BASE}/api/tasks?${params}`, {
            headers: getHeaders()
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        tasks = data.tasks || [];
        renderTaskList();

    } catch (error) {
        console.error('Failed to load tasks:', error);
        showToast('Failed to load tasks: ' + error.message, 'error');
    } finally {
        if (refreshBtnEl) refreshBtnEl.classList.remove('spinning');
    }
}

async function loadTaskDetail(taskId) {
    try {
        const response = await fetch(`${API_BASE}/api/tasks/${taskId}`, {
            headers: getHeaders()
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        return data.task || data;
    } catch (error) {
        console.error('Failed to load task detail:', error);
        return null;
    }
}

function getHeaders() {
    const headers = {
        'Content-Type': 'application/json'
    };
    const token = sessionStorage.getItem('auth_token');
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    return headers;
}

// Rendering
function renderTaskList() {
    if (!taskListEl) return;

    const filteredTasks = filterTasks(tasks);

    if (filteredTasks.length === 0) {
        taskListEl.innerHTML = '<div class="task-list-empty">No tasks found</div>';
        return;
    }

    taskListEl.innerHTML = filteredTasks.map(task => {
        const id = task.task_id || task.taskId || '';
        const goal = task.goal || '';
        const status = task.status || 'pending';
        const priority = task.priority || 0;
        const createdAt = task.created_at || task.createdAt || '';

        return `
        <div class="task-item ${id === selectedTaskId ? 'selected' : ''}"
             data-task-id="${escapeAttr(id)}"
             onclick="selectTask('${escapeAttr(id)}')">
            <div class="task-item-header">
                <span class="task-item-id">${escapeHtml(id.slice(0, 12))}${id.length > 12 ? '...' : ''}</span>
                <span class="task-item-status ${status}">${status}</span>
            </div>
            <div class="task-item-goal">${escapeHtml(goal || 'No goal')}</div>
            <div class="task-item-meta">
                <span class="task-item-priority">P${priority}</span>
                <span>${formatTime(createdAt)}</span>
            </div>
        </div>
    `}).join('');
}

function filterTasks(tasks) {
    if (!statusFilterEl || !statusFilterEl.value) return tasks;
    return tasks.filter(t => t.status === statusFilterEl.value);
}

async function selectTask(taskId) {
    selectedTaskId = taskId;
    renderTaskList();

    const task = await loadTaskDetail(taskId);
    if (task) {
        renderTaskDetailFull(task);
    } else {
        const localTask = tasks.find(t => (t.task_id || t.taskId) === taskId);
        if (localTask) {
            renderTaskDetailFull(localTask);
        }
    }
}

function renderTaskDetail() {
    if (!selectedTaskId) return;
    const task = tasks.find(t => (t.task_id || t.taskId) === selectedTaskId);
    if (task) {
        renderTaskDetailFull(task);
    }
}

function renderTaskDetailFull(task) {
    if (!detailPanelEl) return;

    const id = task.task_id || task.taskId || '';
    const goal = task.goal || '';
    const status = task.status || 'pending';
    const priority = task.priority || 0;
    const createdAt = task.created_at || task.createdAt || '';
    const startedAt = task.started_at || task.startedAt || '';
    const completedAt = task.completed_at || task.completedAt || '';
    const resultBranch = task.result_branch || task.resultBranch || '';
    const subtasks = task.subtasks || [];

    detailPanelEl.innerHTML = `
        <div class="detail-header">
            <div>
                <div class="detail-task-id">${escapeHtml(id)}</div>
                <div class="detail-goal">${escapeHtml(goal || 'No goal')}</div>
                <span class="task-item-status ${status}">${status}</span>
            </div>
        </div>

        <div class="detail-meta">
            <div class="detail-meta-item">
                <span class="detail-meta-label">Priority</span>
                <span class="detail-meta-value">P${priority}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Created</span>
                <span class="detail-meta-value">${formatTime(createdAt)}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Started</span>
                <span class="detail-meta-value">${startedAt ? formatTime(startedAt) : 'Not started'}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Completed</span>
                <span class="detail-meta-value">${completedAt ? formatTime(completedAt) : 'Not completed'}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Result Branch</span>
                <span class="detail-meta-value">${escapeHtml(resultBranch || 'N/A')}</span>
            </div>
        </div>

        <div class="detail-section">
            <h3 class="detail-section-title">Subtasks (${subtasks.length})</h3>
            ${subtasks.length > 0 ? `
            <div class="subtask-list">
                ${subtasks.map(st => {
                    const stId = st.subtask_id || st.subtaskId || '';
                    const stStatus = st.status || 'pending';
                    const stType = st.type || '';
                    const stSummary = st.summary || '';
                    return `
                    <div class="subtask-item">
                        <div class="subtask-status-icon ${stStatus}">
                            ${getSubtaskIcon(stStatus)}
                        </div>
                        <div class="subtask-info">
                            <div class="subtask-description">${escapeHtml(stSummary || 'No summary')}</div>
                            <div class="subtask-type">${escapeHtml(stType)}</div>
                        </div>
                        <span class="subtask-status-text">${stStatus}</span>
                    </div>
                `}).join('')}
            </div>
            ` : '<div class="task-list-empty">No subtasks</div>'}
        </div>
    `;
}

function getSubtaskIcon(status) {
    switch (status) {
        case 'completed': return '✓';
        case 'running': return '●';
        case 'failed': return '✕';
        case 'pending': return '○';
        default: return '○';
    }
}

// Utility Functions
function escapeHtml(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function escapeAttr(str) {
    if (!str) return '';
    return str.replace(/'/g, '\\').replace(/"/g, '\\"');
}

function formatTime(isoString) {
    if (!isoString) return '';
    try {
        const date = new Date(isoString);
        return date.toLocaleString();
    } catch (e) {
        return isoString;
    }
}

function showToast(message, type = 'info') {
    let container = document.querySelector('.toast-container');
    if (!container) {
        container = document.createElement('div');
        container.className = 'toast-container';
        document.body.appendChild(container);
    }

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.remove();
    }, 3000);
}

// Make selectTask globally available
window.selectTask = selectTask;
