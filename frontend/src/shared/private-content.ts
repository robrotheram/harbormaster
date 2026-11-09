// Private transcript/result content is fetched again after its view closes.
// It stays project-scoped while mounted and is never persisted to browser storage.
export const privateContentOptions = { gcTime: 0 } as const;
