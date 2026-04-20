import React from 'react';

type PanelErrorBoundaryProps = {
  panelName: string;
  children: React.ReactNode;
};

type PanelErrorBoundaryState = {
  hasError: boolean;
};

export class PanelErrorBoundary extends React.Component<
  PanelErrorBoundaryProps,
  PanelErrorBoundaryState
> {
  constructor(props: PanelErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): PanelErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error) {
    console.error(`[PanelErrorBoundary] ${this.props.panelName} failed`, error);
  }

  private handleRetry = () => {
    this.setState({ hasError: false });
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="pipboy-box p-6 h-full flex flex-col items-center justify-center gap-2">
          <div className="text-sm radioactive-text">PANEL FAILURE</div>
          <div className="text-xs dim-text text-center max-w-md">
            {this.props.panelName} encountered a rendering error.
          </div>
          <button
            className="px-4 py-2 border-2 border-[#39FF14] text-[#39FF14] text-xs font-mono"
            onClick={this.handleRetry}
            type="button"
          >
            RETRY PANEL
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
