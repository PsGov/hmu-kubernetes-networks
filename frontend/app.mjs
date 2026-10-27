const API = '/api';

export function validateTitle(value) {
  const title = value.trim();
  if (title.length === 0) return 'Enter a task title.';
  if ([...title].length > 120) return 'Use 120 characters or fewer.';
  if (title.includes('\u0000')) return 'Task titles cannot contain a null character.';
  return '';
}

async function request(path, options = {}, expectedStatus = 200) {
  let response;
  try {
    response = await fetch(`${API}${path}`, options);
  } catch {
    throw new Error('Cannot reach the API. Check the connection and try again.');
  }

  if (response.ok && response.status !== expectedStatus) {
    throw new Error('The API returned an unexpected response.');
  }
  // DELETE success has no body. Never try to parse its 204 response as JSON.
  if (response.status === 204) return null;

  let body;
  try {
    body = await response.json();
  } catch {
    throw new Error('The API returned an unreadable response.');
  }

  if (!response.ok) {
    const message = body && typeof body.message === 'string' &&
      typeof body.error === 'string' ? body.message : 'The request failed.';
    throw new Error(message);
  }
  return body;
}

function isTask(task) {
  return task && typeof task.id === 'string' &&
    typeof task.title === 'string' && typeof task.created_at === 'string';
}

function setState(element, message, isError = false) {
  element.textContent = message;
  element.classList.toggle('error', isError);
}

const form = document.querySelector('#task-form');
const input = document.querySelector('#task-title');
const addButton = document.querySelector('#add-task');
const reloadTasksButton = document.querySelector('#reload-tasks');
const taskFeedback = document.querySelector('#task-feedback');
const taskState = document.querySelector('#task-state');
const taskList = document.querySelector('#task-list');
const reloadIdentityButton = document.querySelector('#reload-identity');
const identityState = document.querySelector('#identity-state');
const identityDetails = document.querySelector('#identity-details');

let tasksLoading = false;
let mutationRunning = false;

function renderTasks(tasks) {
  taskList.replaceChildren();
  for (const task of tasks) {
    const item = document.createElement('li');
    const title = document.createElement('span');
    title.className = 'task-title';
    title.textContent = task.title;
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'secondary';
    button.textContent = 'Delete';
    button.setAttribute('aria-label', `Delete task: ${task.title}`);
    button.addEventListener('click', () => deleteTask(task.id));
    item.append(title, button);
    taskList.append(item);
  }
  taskList.hidden = tasks.length === 0;
  setState(taskState, tasks.length === 0 ? 'No tasks yet. Add one above.' : '');
}

async function loadTasks() {
  if (tasksLoading) return;
  tasksLoading = true;
  reloadTasksButton.disabled = true;
  addButton.disabled = true;
  taskList.hidden = true;
  setState(taskState, 'Loading tasks…');
  try {
    const body = await request('/tasks');
    if (!body || !Array.isArray(body.tasks) || !body.tasks.every(isTask)) {
      throw new Error('The API returned an unexpected task list.');
    }
    renderTasks(body.tasks);
  } catch (error) {
    setState(taskState, `Could not load tasks: ${error.message}`, true);
  } finally {
    tasksLoading = false;
    reloadTasksButton.disabled = false;
    if (!mutationRunning) addButton.disabled = false;
  }
}

async function loadIdentity() {
  reloadIdentityButton.disabled = true;
  identityDetails.hidden = true;
  setState(identityState, 'Loading API identity…');
  try {
    const body = await request('/whoami');
    const fields = ['pod_name', 'pod_namespace', 'node_name', 'pod_ip'];
    if (!body || !fields.every(key => typeof body[key] === 'string')) {
      throw new Error('The API returned unexpected identity details.');
    }
    document.querySelector('#pod-name').textContent = body.pod_name;
    document.querySelector('#pod-namespace').textContent = body.pod_namespace;
    document.querySelector('#node-name').textContent = body.node_name;
    document.querySelector('#pod-ip').textContent = body.pod_ip;
    identityDetails.hidden = false;
    setState(identityState, '');
  } catch (error) {
    setState(identityState, `Could not load API identity: ${error.message}`, true);
  } finally {
    reloadIdentityButton.disabled = false;
  }
}

async function addTask(event) {
  event.preventDefault();
  if (mutationRunning || tasksLoading) return;
  const title = input.value.trim();
  const validationError = validateTitle(input.value);
  if (validationError) {
    setState(taskFeedback, validationError, true);
    input.focus();
    return;
  }

  mutationRunning = true;
  addButton.disabled = true;
  setState(taskFeedback, 'Adding task…');
  try {
    const task = await request('/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title }),
    }, 201);
    if (!isTask(task)) throw new Error('The API returned an unexpected task.');
    input.value = '';
    setState(taskFeedback, 'Task added.');
    await loadTasks();
  } catch (error) {
    setState(taskFeedback, `Could not add task: ${error.message}`, true);
  } finally {
    mutationRunning = false;
    addButton.disabled = false;
  }
}

async function deleteTask(id) {
  if (mutationRunning || tasksLoading) return;
  mutationRunning = true;
  addButton.disabled = true;
  setState(taskFeedback, 'Deleting task…');
  try {
    await request(`/tasks/${encodeURIComponent(id)}`, { method: 'DELETE' }, 204);
    setState(taskFeedback, 'Task deleted.');
    await loadTasks();
  } catch (error) {
    setState(taskFeedback, `Could not delete task: ${error.message}`, true);
  } finally {
    mutationRunning = false;
    addButton.disabled = false;
  }
}

form.addEventListener('submit', addTask);
reloadTasksButton.addEventListener('click', loadTasks);
reloadIdentityButton.addEventListener('click', loadIdentity);
loadTasks();
loadIdentity();
