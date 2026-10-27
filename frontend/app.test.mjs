import assert from 'node:assert/strict';
import test from 'node:test';

class Element {
  constructor() {
    this.children = [];
    this.listeners = new Map();
    this.classList = { toggle: (name, enabled) => { this.error = name === 'error' && enabled; } };
    this.textContent = '';
    this.hidden = false;
    this.disabled = false;
    this.value = '';
  }
  addEventListener(name, listener) { this.listeners.set(name, listener); }
  append(...children) { this.children.push(...children); }
  replaceChildren() { this.children = []; }
  setAttribute(name, value) { this[name] = value; }
  focus() { this.focused = true; }
  trigger(name, event = {}) { return this.listeners.get(name)(event); }
}

const ids = [
  'task-form', 'task-title', 'add-task', 'reload-tasks', 'task-feedback',
  'task-state', 'task-list', 'reload-identity', 'identity-state',
  'identity-details', 'pod-name', 'pod-namespace', 'node-name', 'pod-ip',
];
const elements = Object.fromEntries(ids.map(id => [id, new Element()]));
globalThis.document = {
  querySelector(selector) { return elements[selector.slice(1)]; },
  createElement() { return new Element(); },
};

const calls = [];
const task = {
  id: '550e8400-e29b-41d4-a716-446655440000',
  title: '<img src=x onerror=alert(1)>',
  created_at: '2026-01-01T12:00:00Z',
};
let tasks = [];
let taskError = false;
globalThis.fetch = async (url, options = {}) => {
  calls.push({ url, options });
  if (url === '/api/whoami') {
    return Response.json({ pod_name: 'api-1', pod_namespace: 'kubelab', node_name: 'node-1', pod_ip: '10.0.0.1' });
  }
  if (url === '/api/tasks' && options.method === 'POST') {
    const body = JSON.parse(options.body);
    if (body.title === 'bad') {
      return Response.json({ error: 'validation_error', message: 'Rejected by API' }, { status: 400 });
    }
    tasks = [task];
    return Response.json(task, { status: 201 });
  }
  if (url === `/api/tasks/${task.id}` && options.method === 'DELETE') {
    tasks = [];
    return new Response(null, { status: 204 });
  }
  if (url === '/api/tasks') {
    if (taskError) return Response.json({ error: 'service_unavailable', message: 'Worker unavailable' }, { status: 503 });
    return Response.json({ tasks });
  }
  throw new Error(`Unexpected URL: ${url}`);
};

const { validateTitle } = await import('./app.mjs');
const tick = () => new Promise(resolve => setImmediate(resolve));

test('initial requests show empty list and API identity', async () => {
  await tick();
  assert.deepEqual(calls.slice(0, 2).map(call => call.url), ['/api/tasks', '/api/whoami']);
  assert.equal(elements['task-state'].textContent, 'No tasks yet. Add one above.');
  assert.equal(elements['task-list'].hidden, true);
  assert.equal(elements['pod-name'].textContent, 'api-1');
  assert.equal(elements['node-name'].textContent, 'node-1');
  assert.equal(elements['identity-details'].hidden, false);
});

test('title validation counts Unicode code points and blocks invalid text', () => {
  assert.equal(validateTitle('  '), 'Enter a task title.');
  assert.equal(validateTitle('😀'.repeat(120)), '');
  assert.equal(validateTitle('😀'.repeat(121)), 'Use 120 characters or fewer.');
  assert.match(validateTitle('a\u0000b'), /null character/);
});

test('create renders title as text and delete handles an empty 204', async () => {
  elements['task-title'].value = '  Learn Services  ';
  await elements['task-form'].trigger('submit', { preventDefault() {} });
  assert.equal(JSON.parse(calls.find(call => call.options.method === 'POST').options.body).title, 'Learn Services');
  assert.equal(elements['task-list'].children[0].children[0].textContent, task.title);
  assert.equal(elements['task-list'].children[0].children[0].children.length, 0);
  assert.equal(elements['task-feedback'].textContent, 'Task added.');

  await elements['task-list'].children[0].children[1].trigger('click');
  assert.equal(elements['task-feedback'].textContent, 'Task deleted.');
  assert.equal(elements['task-list'].children.length, 0);
  assert.equal(elements['task-state'].textContent, 'No tasks yet. Add one above.');
});

test('API errors are shown without clearing the input', async () => {
  elements['task-title'].value = 'bad';
  await elements['task-form'].trigger('submit', { preventDefault() {} });
  assert.match(elements['task-feedback'].textContent, /Rejected by API/);
  assert.equal(elements['task-feedback'].error, true);
  assert.equal(elements['task-title'].value, 'bad');

  taskError = true;
  await elements['reload-tasks'].trigger('click');
  assert.match(elements['task-state'].textContent, /Worker unavailable/);
  assert.equal(elements['task-list'].hidden, true);
});
