import { ReactFlowProvider } from '@xyflow/react';
import { GraphExplorer } from './features/graph/GraphExplorer';

export function App() {
  return (
    <ReactFlowProvider>
      <GraphExplorer />
    </ReactFlowProvider>
  );
}
