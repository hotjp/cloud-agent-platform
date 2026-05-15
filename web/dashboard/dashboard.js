/**
 * Cloud Agent Platform Dashboard
 * Real-time task monitoring via WebSocket
 */

// API Configuration - adjust these for your environment
const API_BASE = window.location.origin;
const WS_BASE = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_BASE}//${window.location.host}/api/v1/ws`;

// State
let tasks = [];
let selectedTaskId = null;
let ws = null;
let wsConnected = false;
let wsReconnectTimeout = null;

// DOM Elements
const taskListEl = document.getElementById('taskList');
const detailPanelEl = document.getElementById('detailPanel');
const statusFilterEl = document.getElementById('statusFilter');
const refreshBtnEl = document.getElementById('refreshBtn');
const connectionStatusEl = document.getElementById('connectionStatus');

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    initWebSocket();
    loadTasks();
    setupEventListeners();
});

function setupEventListeners() {
    refreshBtnEl.addEventListener('click', () => {
        loadTasks();
    });

    statusFilterEl.addEventListener('change', () => {
        renderTaskList();
    });
}

// WebSocket Connection
function initWebSocket() {
    updateConnectionStatus('connecting');

    ws = new WebSocket(WS_URL);

    ws.onopen = () => {
        console.log('WebSocket connected');
        wsConnected = true;
        updateConnectionStatus('connected');
        showToast('Connected to server', 'success');
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
}

function scheduleReconnect() {
    if (wsReconnectTimeout) {
        clearTimeout(wsReconnectTimeout);
    }
    wsReconnectTimeout = setTimeout(() => {
        console.log('Attempting to reconnect...');
        initWebSocket();
    }, 3000);
}

function handleWSEvent(event) {
    console.log('WS Event:', event);

    switch (event.type) {
        case 'task.status_changed':
            handleTaskStatusChanged(event);
            break;
        case 'task.created':
            handleTaskCreated(event);
            break;
        case 'task.approval_required':
            handleApprovalRequired(event);
            break;
        case 'agent.thought':
            handleAgentThought(event);
            break;
        case 'artifact.created':
            handleArtifactCreated(event);
            break;
        case 'auth_success':
            console.log('WebSocket authenticated');
            break;
        case 'subscribed':
            console.log('Subscribed to task:', event.taskId);
            break;
        default:
            console.log('Unknown event type:', event.type);
    }
}

function handleTaskStatusChanged(event) {
    const taskId = event.taskId;
    const payload = event.payload ? JSON.parse(event.payload) : {};

    const task = tasks.find(t => t.taskId === taskId);
    if (task) {
        task.status = payload.new || event.status;
        renderTaskList();

        if (selectedTaskId === taskId) {
            renderTaskDetail();
        }

        showToast(`Task ${taskId.slice(0, 8)}... status: ${task.status}`, 'info');
    }
}

function handleTaskCreated(event) {
    loadTasks();
}

function handleApprovalRequired(event) {
    showToast('Approval required for task', 'warning');
}

function handleAgentThought(event) {
    // Update logs for the selected task
    if (selectedTaskId === event.taskId) {
        renderTaskDetail();
    }
}

function handleArtifactCreated(event) {
    if (selectedTaskId === event.taskId) {
        renderTaskDetail();
    }
}

function updateConnectionStatus(status) {
    const dot = connectionStatusEl.querySelector('.status-dot');
    const text = connectionStatusEl.querySelector('.status-text');

    dot.className = 'status-dot ' + status;

    switch (status) {
        case 'connected':
            text.textContent = 'Connected';
            break;
        case 'connecting':
            text.textContent = 'Connecting...';
            break;
        case 'disconnected':
            text.textContent = 'Disconnected';
            break;
    }
}

// API Calls
async function loadTasks() {
    try {
        refreshBtnEl.classList.add('spinning');

        const params = new URLSearchParams();
        params.append('page', '1');
        params.append('pageSize', '100');

        const status = statusFilterEl.value;
        if (status) {
            params.append('status', status);
        }

        const response = await fetch(`${API_BASE}/api.v1/tasks?${params}`, {
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
        showToast('Failed to load tasks', 'error');
    } finally {
        refreshBtnEl.classList.remove('spinning');
    }
}

async function loadTaskDetail(taskId) {
    try {
        const response = await fetch(`${API_BASE}/api.v1/tasks/${taskId}`, {
            headers: getHeaders()
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        return data.task;

    } catch (error) {
        console.error('Failed to load task detail:', error);
        return null;
    }
}

function getHeaders() {
    const headers = {
        'Content-Type': 'application/json'
    };

    // Include auth token if available
    const token = sessionStorage.getItem('auth_token');
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }

    return headers;
}

// Rendering
function renderTaskList() {
    const filteredTasks = filterTasks(tasks);
    const status = statusFilterEl.value;

    if (filteredTasks.length === 0) {
        taskListEl.innerHTML = '<div class="task-list-empty">No tasks found</div>';
        return;
    }

    taskListEl.innerHTML = filteredTasks.map(task => `
        <div class="task-item ${task.taskId === selectedTaskId ? 'selected' : ''}"
             data-task-id="${task.taskId}"
             onclick="selectTask('${task.taskId}')">
            <div class="task-item-header">
                <span class="task-item-id">${escapeHtml(task.taskId.slice(0, 12))}...</span>
                <span class="task-item-status ${task.status}">${task.status}</span>
            </div>
            <div class="task-item-goal">${escapeHtml(task.goal || 'No goal')}</div>
            <div class="task-item-meta">
                <span class="task-item-priority">
                    P${task.priority || 0}
                </span>
                <span>${formatTime(task.createdAt)}</span>
                <div class="task-item-progress">
                    <div class="task-item-progress-bar" style="width: ${task.progress || 0}%"></div>
                </div>
            </div>
        </div>
    `).join('');
}

function filterTasks(tasks) {
    const status = statusFilterEl.value;
    if (!status) return tasks;
    return tasks.filter(t => t.status === status);
}

async function selectTask(taskId) {
    selectedTaskId = taskId;
    renderTaskList();

    // Load full task details
    const task = await loadTaskDetail(taskId);
    if (task) {
        renderTaskDetailFull(task);
    } else {
        // Fallback to local data
        const localTask = tasks.find(t => t.taskId === taskId);
        if (localTask) {
            renderTaskDetailFull(localTask);
        }
    }
}

function renderTaskDetailFull(task) {
    const startedAt = task.startedAt ? formatTime(task.startedAt) : 'Not started';
    const completedAt = task.completedAt ? formatTime(task.completedAt) : 'Not completed';

    detailPanelEl.innerHTML = `
        <div class="detail-header">
            <div>
                <div class="detail-task-id">${escapeHtml(task.taskId)}</div>
                <div class="detail-goal">${escapeHtml(task.goal || 'No goal')}</div>
                <span class="task-item-status ${task.status}">${task.status}</span>
            </div>
        </div>

        <div class="detail-meta">
            <div class="detail-meta-item">
                <span class="detail-meta-label">Priority</span>
                <span class="detail-meta-value">P${task.priority || 0}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Progress</span>
                <span class="detail-meta-value">${task.progress || 0}%</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Created</span>
                <span class="detail-meta-value">${formatTime(task.createdAt)}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Started</span>
                <span class="detail-meta-value">${startedAt}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Completed</span>
                <span class="detail-meta-value">${completedAt}</span>
            </div>
            <div class="detail-meta-item">
                <span class="detail-meta-label">Result Branch</span>
                <span class="detail-meta-value">${escapeHtml(task.resultBranch || 'N/A')}</span>
            </div>
        </div>

        <div class="detail-section">
            <h3 class="detail-section-title">Statistics</h3>
            <div class="detail-stats">
                <div class="detail-stat">
                    <div class="detail-stat-label">Tokens Used</div>
                    <div class="detail-stat-value">${formatNumber(task.tokensUsed || 0)}</div>
                </div>
                <div class="detail-stat">
                    <div class="detail-stat-label">Agents Used</div>
                    <div class="detail-stat-value">${task.agentsUsed || 0}</div>
                </div>
                <div class="detail-stat">
                    <div class="detail-stat-label">Est. Cost</div>
                    <div class="detail-stat-value">¥${(task.estimatedCost || 0).toFixed(4)}</div>
                </div>
                <div class="detail-stat">
                    <div class="detail-stat-label">Subtasks</div>
                    <div class="detail-stat-value">${task.subtasks?.length || 0}</div>
                </div>
            </div>
        </div>

        ${task.subtasks && task.subtasks.length > 0 ? `
        <div class="detail-section">
            <h3 class="detail-section-title">Subtasks</h3>
            <div class="subtask-list">
                ${task.subtasks.map(st => `
                    <div class="subtask-item">
                        <div class="subtask-status-icon ${st.status}">
                            ${getSubtaskIcon(st.status)}
                        </div>
                        <div class="subtask-info">
                            <div class="subtask-description">${escapeHtml(st.description || 'No description')}</div>
                            <div class="subtask-type">${st.type || 'unknown'}</div>
                        </div>
                        <span class="subtask-status-text">${st.status}</span>
                    </div>
                `).join('')}
            </div>
        </div>
        ` : ''}

        ${task.logs && task.logs.length > 0 ? `
        <div class="detail-section">
            <h3 class="detail-section-title">Recent Logs</h3>
            <div class="log-list">
                ${task.logs.slice(-10).map(log => `
                    <div class="log-item">
                        <div class="log-item-header">
                            <span class="log-item-time">${formatTime(log.timestamp)}</span>
                            <span class="log-item-level ${log.level || 'info'}">${log.level || 'info'}</span>
                        </div>
                        <div class="log-item-message ${log.level === 'error' ? 'error' : ''}">${escapeHtml(log.message || '')}</div>
                    </div>
                `).join('')}
            </div>
        </div>
        ` : ''}
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

function formatTime(isoString) {
    if (!isoString) return '';
    const date = new Date(isoString);
    return date.toLocaleString();
}

function formatNumber(num) {
    if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M';
    }
    if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
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