import { defineConfig } from 'vite';
export default defineConfig({root:'web',build:{outDir:'../dist',emptyOutDir:true},server:{proxy:{'/api':{target:'http://127.0.0.1:17381',ws:true}}}});