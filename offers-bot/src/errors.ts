/**
 * Текст ошибки для человека.
 *
 * В боте тридцать три места писали `(e as Error).message`, и это опасно: бросят
 * строку или объект без `message` — человек увидит «undefined» вместо причины.
 * Одна точка на все места: `message` у Error, иначе строковое представление.
 */
export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message || e.name;
  if (typeof e === 'string') return e;
  try {
    return JSON.stringify(e);
  } catch {
    return String(e);
  }
}