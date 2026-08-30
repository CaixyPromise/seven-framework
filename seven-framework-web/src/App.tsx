import { useEffect, useMemo, useState } from 'react';
import { RouterProvider } from 'react-router-dom';
import {
  SAFE_RUNTIME_FEATURES,
  getRuntimeFeatures,
} from '@/api/runtimeFeaturesController';
import type { RuntimeFeatures } from '@/lib/http/types';
import { createRuntimeRouter } from './router';

function App() {
  const [features, setFeatures] = useState<RuntimeFeatures | null>(null);
  const router = useMemo(() => (features ? createRuntimeRouter(features) : null), [features]);

  useEffect(() => {
    let active = true;
    getRuntimeFeatures()
      .then((resolvedFeatures) => {
        if (active) {
          setFeatures(resolvedFeatures);
        }
      })
      .catch(() => {
        if (active) {
          setFeatures(SAFE_RUNTIME_FEATURES);
        }
      });
    return () => {
      active = false;
    };
  }, []);

  if (!router) {
    return <main aria-busy="true" aria-label="正在加载" style={{ minHeight: '100vh' }} />;
  }

  return <RouterProvider router={router} />;
}

export default App;
