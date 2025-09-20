# Руководство по исправлению ошибок ESLint

## Быстрые команды

```bash
# Проверка всех файлов
cd frontend && yarn lint

# Проверка конкретного файла
cd frontend && yarn eslint src/views/YourFile.vue

# Автоматическое исправление
cd frontend && yarn eslint --fix src/views/YourFile.vue

# Проверка только ошибок (без предупреждений)
cd frontend && yarn eslint --quiet src/views/YourFile.vue
```

## Частые ошибки и их исправления

### 1. Синтаксические ошибки
**Ошибка**: `Parsing error: Unexpected token`
**Причина**: Комментарии слипаются с кодом
```javascript
// ❌ Неправильно:
// Комментарийкод

// ✅ Правильно:
// Комментарий
код
```

### 2. Лишние пробелы
**Ошибка**: `Trailing spaces not allowed`
**Решение**: Удалить пробелы в конце строки
```vue
<!-- ❌ Неправильно: -->
<b-select 

<!-- ✅ Правильно: -->
<b-select
```

### 3. Операторы инкремента
**Ошибка**: `Unary operator '++' used`
**Решение**: Заменить на `+= 1`
```javascript
// ❌ Неправильно:
counter++;

// ✅ Правильно:
counter += 1;
```

### 4. Стрелочные функции
**Ошибка**: `Expected parentheses around arrow function argument`
**Решение**: Добавить скобки вокруг аргумента
```javascript
// ❌ Неправильно:
items.forEach(item => {

// ✅ Правильно:
items.forEach((item) => {
```

### 5. Цепочки методов
**Ошибка**: `Expected line break before`
**Решение**: Добавить перенос строки
```javascript
// ❌ Неправильно:
promise.then().catch();

// ✅ Правильно:
promise.then()
  .catch();
```

### 6. Запятые в массивах
**Ошибка**: `Missing trailing comma`
**Решение**: Добавить запятую в конце
```javascript
// ❌ Неправильно:
const items = [
  'item1',
  'item2'
];

// ✅ Правильно:
const items = [
  'item1',
  'item2',
];
```

### 7. Возвращаемые значения
**Ошибка**: `Expected to return a value`
**Решение**: Добавить явный return
```javascript
// ❌ Неправильно:
getValue() {
  if (condition) {
    return value;
  }
  // Нет return для else
}

// ✅ Правильно:
getValue() {
  if (condition) {
    return value;
  }
  return defaultValue;
}
```

### 8. Отступы
**Ошибка**: `Expected indentation of X spaces but found Y`
**Решение**: Исправить отступы (обычно 2 или 4 пробела)
```javascript
// ❌ Неправильно:
if (condition) {
  doSomething();
}

// ✅ Правильно (2 пробела):
if (condition) {
  doSomething();
}
```

## Автоматическое исправление

ESLint может автоматически исправить многие ошибки:

```bash
# Исправить все возможные ошибки
yarn eslint --fix src/views/YourFile.vue

# Исправить с проверкой
yarn eslint --fix --quiet src/views/YourFile.vue
```

## Настройка IDE

### VS Code
Установите расширение ESLint и добавьте в настройки:
```json
{
  "eslint.autoFixOnSave": true,
  "eslint.validate": [
    "javascript",
    "vue"
  ]
}
```

### WebStorm
Включите ESLint в настройках: `File > Settings > Languages & Frameworks > JavaScript > Code Quality Tools > ESLint`

## Проверка перед коммитом

Добавьте в `package.json`:
```json
{
  "scripts": {
    "lint:check": "eslint --ext .js,.vue src",
    "lint:fix": "eslint --ext .js,.vue --fix src"
  }
}
```

Запускайте перед каждым коммитом:
```bash
yarn lint:check
```

## Полезные правила ESLint

- `no-console` - запрещает console.log (можно отключить для отладки)
- `no-unused-vars` - запрещает неиспользуемые переменные
- `no-trailing-spaces` - запрещает пробелы в конце строки
- `indent` - проверяет отступы
- `quotes` - проверяет тип кавычек
- `semi` - проверяет точки с запятой

---

*Создано: 2025-01-15*
*Для проекта: Listmonk*
