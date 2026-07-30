import { describe, expect, it } from 'vitest';
import { registrationError } from './registration';

describe('registrationError', () => {
  it('requires a username', () => {
    expect(registrationError('   ', 'strong-pass-123', 'strong-pass-123')).toBe('Укажи логин владельца.');
  });

  it('requires a password of at least 10 characters', () => {
    expect(registrationError('admin', 'short', 'short')).toBe('Пароль должен быть минимум 10 символов.');
    expect(registrationError('admin', '123456789', '123456789')).toBe('Пароль должен быть минимум 10 символов.');
  });

  it('requires matching password confirmation', () => {
    expect(registrationError('admin', 'strong-pass-123', 'strong-pass-124')).toBe('Пароли не совпадают.');
  });

  it('accepts valid input', () => {
    expect(registrationError('admin', 'strong-pass-123', 'strong-pass-123')).toBeNull();
    expect(registrationError('  admin  ', '1234567890', '1234567890')).toBeNull();
  });
});
