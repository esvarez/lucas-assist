const globalScope = globalThis as Record<string, unknown>
if (typeof globalScope.global === 'undefined') {
  globalScope.global = globalThis
}
