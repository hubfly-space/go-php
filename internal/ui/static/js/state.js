// Simple reactive state store

window.Store = (function () {
  const state = {
    status: null,
    sites: [],
    config: null,
    runtimes: [],
    logs: [],
    system: null,
    loading: {},
  };

  const listeners = {};

  function get(key) {
    return state[key];
  }

  function set(key, value) {
    state[key] = value;
    emit(key, value);
  }

  function on(key, fn) {
    if (!listeners[key]) listeners[key] = [];
    listeners[key].push(fn);
    return () => { listeners[key] = listeners[key].filter(f => f !== fn); };
  }

  function emit(key, value) {
    (listeners[key] || []).forEach(fn => fn(value));
    (listeners['*'] || []).forEach(fn => fn(key, value));
  }

  function setLoading(key, loading) {
    state.loading[key] = loading;
    emit('loading', state.loading);
  }

  function isLoading(key) {
    return !!state.loading[key];
  }

  return { get, set, on, emit, setLoading, isLoading };
})();
