import NewProjectDialog from '../components/NewProjectDialog'

function ProjectsPage() {
  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">Projects</h1>
        <NewProjectDialog />
      </div>
    </div>
  )
}

export default ProjectsPage
