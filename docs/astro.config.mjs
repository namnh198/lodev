import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://namnh198.github.io',
  base: '/lodev',
  integrations: [
    starlight({
      title: 'lodev',
      logo: { src: './logo/lodev-logo.svg', replacesTitle: true },
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/namnh198/lodev' }],
      editLink: { baseUrl: 'https://github.com/namnh198/lodev/edit/main/docs/' },
    }),
  ],
});
