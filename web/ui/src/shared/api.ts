export class APIError extends Error {
  status: number;
  detail: string | undefined;

  constructor(status: number, message: string, detail?: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.detail = detail;
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
  });

  if (!res.ok) {
    let title = res.statusText;
    let detail: string | undefined;
    try {
      const body = (await res.json()) as {
        title?: string;
        error?: string;
        detail?: string;
        errors?: { message?: string }[];
      };
      // huma puts the underlying error in errors[0].message, not detail
      const firstErr = body.errors?.[0]?.message;
      title = firstErr ?? body.detail ?? body.title ?? body.error ?? title;
      detail = firstErr ?? body.detail;
    } catch {
      // ignore parse error, use statusText
    }
    throw new APIError(res.status, title, detail);
  }

  // 204 No Content — never has a body
  if (res.status === 204) return undefined as T;

  // 202 Accepted — may or may not have a body
  if (res.status === 202) {
    const text = await res.text();
    if (!text) return undefined as T;
    return JSON.parse(text) as T;
  }

  return res.json() as Promise<T>;
}
