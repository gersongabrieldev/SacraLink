import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import { Api } from './lib/api';
import { StoreProvider, resolveServerURL } from './lib/store';
import { App } from './App';

resolveServerURL().then((url) => {
  const api = new Api(url);
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <StoreProvider api={api}>
        <App />
      </StoreProvider>
    </StrictMode>,
  );
});
