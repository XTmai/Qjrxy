// 前端静态检查：捕获「引用了但未定义的函数」「字段不匹配」这类低级错误。
// 用法：node scripts/check-frontend.mjs
import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';

const ASSET_DIR = 'web/dist/assets';
let failed = 0;

const files = fs.readdirSync(ASSET_DIR).filter((f) => f.endsWith('.js'));

// 1. 语法检查
console.log('== 1. 语法检查 ==');
for (const f of files) {
  const full = path.join(ASSET_DIR, f);
  try {
    execSync(`node --check "${full}"`, { stdio: 'pipe' });
    console.log('  OK   ' + f);
  } catch (e) {
    console.log('  FAIL ' + f);
    console.log('       ' + String(e.stderr || e.message).split('\n')[0]);
    failed++;
  }
}

// 2. 未定义函数：本文件调用的驼峰函数，若既没定义也没从 import 引入
console.log('\n== 2. 未定义的函数调用 ==');
const BUILTIN = new Set([
  'Boolean','Date','Error','Event','FormData','JSON','Map','Math','Number','Object',
  'Promise','RegExp','Set','String','URLSearchParams','Array','AbortController',
  'decodeURIComponent','encodeURIComponent','fetch','parseFloat','parseInt','isNaN',
  'setTimeout','setInterval','clearTimeout','clearInterval','structuredClone',
  'requestAnimationFrame','cancelAnimationFrame','queueMicrotask',
]);

for (const f of files) {
  const src = fs.readFileSync(path.join(ASSET_DIR, f), 'utf8');

  const defined = new Set();
  for (const m of src.matchAll(/(?:^|\n)\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)/g)) {
    defined.add(m[1]);
  }
  for (const m of src.matchAll(/(?:^|\n)\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=/g)) {
    defined.add(m[1]);
  }
  const imported = new Set();
  for (const m of src.matchAll(/import\s*\{([^}]+)\}\s*from/g)) {
    m[1].split(',').forEach((s) => {
      const name = s.trim().split(/\s+as\s+/).pop();
      if (name) imported.add(name);
    });
  }

  const missing = [];
  for (const m of src.matchAll(/(?<![.\w$])([a-z][A-Za-z0-9$]*[A-Z][\w$]*)\s*\(/g)) {
    const name = m[1];
    if (defined.has(name) || imported.has(name) || BUILTIN.has(name)) continue;
    if (!missing.includes(name)) missing.push(name);
  }
  if (missing.length) {
    console.log('  WARN ' + f + ' 可能未定义: ' + missing.join(', '));
    failed++;
  } else {
    console.log('  OK   ' + f);
  }
}

// 3. 前端提交字段 vs 后端结构体（防 unknown field）
console.log('\n== 3. 前端字段 vs 后端 AccountInput ==');
try {
  const go = fs.readFileSync('internal/service/account.go', 'utf8');
  const block = go.split('type AccountInput struct')[1].split('\n}')[0];
  const be = new Set([...block.matchAll(/json:"(\w+)"/g)].map((m) => m[1]));

  const js = fs.readFileSync(path.join(ASSET_DIR, 'page-accounts.js'), 'utf8');
  const m = js.match(/const payload = \{([\s\S]*?)\n\s*\};/);
  if (!m) {
    console.log('  WARN 未找到 payload 定义');
  } else {
    const fe = new Set([...m[1].matchAll(/^\s*(\w+):/gm)].map((x) => x[1]));
    const extra = [...fe].filter((x) => !be.has(x));
    if (extra.length) {
      console.log('  FAIL 前端多出字段（会报 unknown field）: ' + extra.join(', '));
      failed++;
    } else {
      console.log('  OK   字段一致（' + fe.size + ' 个）');
    }
  }
} catch (e) {
  console.log('  WARN 检查失败: ' + e.message);
}

console.log('\n' + (failed === 0 ? '全部通过' : `发现 ${failed} 个问题`));
process.exit(failed === 0 ? 0 : 1);
