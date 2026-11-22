// Fails the build on a broken internal link or anchor in the generated site.
import { readdirSync, readFileSync, existsSync, statSync } from 'node:fs';
import path from 'node:path';

const dist = path.resolve('dist');
const base = '';
const pages = (dir) =>
  readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? (e.name === 'pagefind' ? [] : pages(path.join(dir, e.name))) : e.name.endsWith('.html') ? [path.join(dir, e.name)] : [],
  );
const idCache = new Map();
const idsOf = (file) => {
  if (!idCache.has(file)) idCache.set(file, new Set([...readFileSync(file, 'utf8').matchAll(/\sid="([^"]+)"/g)].map((m) => m[1])));
  return idCache.get(file);
};

const errors = [];
const sidebarOf = (html) => (html.match(/<details[\s\S]*?<\/details>/g) ?? []).filter((d) => d.includes('sidebar-'));
for (const file of pages(dist)) {
  const page = path.relative(dist, file);
  const html = readFileSync(file, 'utf8');
  if (page === path.join('commands', 'index.html')) {
    for (const d of sidebarOf(html)) if (!/<a /.test(d)) errors.push(`${page}: an empty sidebar group (autogenerate matched no pages)`);
  }
  for (const [, raw] of html.matchAll(/\shref="([^"]*)"/g)) {
    if (/^([a-z][a-z0-9+.-]*:|\/\/)/i.test(raw)) continue; // external
    const [pathname, fragment] = raw.split('#');
    let target = file;
    if (pathname) {
      if (!pathname.startsWith(base + '/')) { errors.push(`${page}: ${raw} (outside the site root)`); continue; }
      target = path.join(dist, pathname.slice(base.length).split('?')[0]);
      if (existsSync(target) && statSync(target).isDirectory()) target = path.join(target, 'index.html');
      if (!existsSync(target)) { errors.push(`${page}: ${raw} (no such page)`); continue; }
    }
    if (fragment && target.endsWith('.html') && !idsOf(target).has(decodeURIComponent(fragment))) errors.push(`${page}: ${raw} (no such anchor)`);
  }
}
if (errors.length) {
  console.error(`${errors.length} broken link(s):\n` + errors.join('\n'));
  process.exit(1);
}
console.log('links ok');
