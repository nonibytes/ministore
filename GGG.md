what happens if worker makes a mistake and verifier has to give it feedback?



Also, right now, there's no real way to prevent workers from modifying these task files. Here's an idea, what if instead of storing these tasks directly on file in project, we use a cli tool?



Each agent that is spun gets a "code" and it can use that code when it's updating tasks. That way permissions can be automatically baked in as workers cannot mistakenly update what they don't have permissions to do.



To make things even better, we can avoid filesystem altogether and simply store the tasks in sqlite. We can still get the ability to serach and filter even in sqlite. This ensure we have clean separation for every project. So worker's job is just to communicate with this task traking cli.





You can take inspiration from the ministore cli's approach/design allowing for flexible search, eyword serachess and much more.



The main difference is that this time, we are keeping the entire history of project changes in this sqlite file using some for of git in database system. Golang has a nice library for this sort of stuff (git in regular apps/databases without file system!).





You should write a design doc for this cli too just like the ministore one provioded.