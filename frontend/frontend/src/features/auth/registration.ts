// Валидация first-run регистрации владельца. Возвращает текст ошибки
// для показа в форме или null, если всё заполнено верно.
export function registrationError(username: string, password: string, confirmPassword: string): string | null {
  if (!username.trim()) return 'Укажи логин владельца.';
  if (password.length < 10) return 'Пароль должен быть минимум 10 символов.';
  if (password !== confirmPassword) return 'Пароли не совпадают.';
  return null;
}
