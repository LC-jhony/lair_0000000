import type { LairController } from '../../src/useLair';
import type { PaneId } from '../../src/panes/types';

export interface TerminalPaneProps {
  state: LairController;
  onNavigate?: (id: PaneId) => void;
}
